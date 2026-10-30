package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/EstebanForge/lite-serve/internal/caddy"
	"github.com/EstebanForge/lite-serve/internal/home"
	"github.com/EstebanForge/lite-serve/internal/proc"
	"github.com/EstebanForge/lite-serve/internal/registry"
	"github.com/EstebanForge/lite-serve/internal/session"
	"github.com/EstebanForge/lite-serve/internal/site"
)

type doctorStatus string

const (
	doctorOK   doctorStatus = "OK"
	doctorWarn doctorStatus = "WARN"
	doctorFail doctorStatus = "FAIL"
)

type doctorCheck struct {
	Status doctorStatus
	Name   string
	Detail string
}

type doctorReport []doctorCheck

func (r doctorReport) Failed() bool {
	for _, check := range r {
		if check.Status == doctorFail {
			return true
		}
	}
	return false
}

func (r doctorReport) Write(w io.Writer) {
	for _, check := range r {
		fmt.Fprintf(w, "%-4s %-18s %s\n", check.Status, check.Name, check.Detail)
	}
}

func runDoctor() error {
	h, err := home.Dir()
	if err != nil {
		return err
	}
	report := doctorReport{}
	add := func(status doctorStatus, name, detail string) {
		report = append(report, doctorCheck{Status: status, Name: name, Detail: detail})
	}

	platform := runtime.GOOS + "-" + runtime.GOARCH
	if platform == "linux-amd64" || platform == "darwin-arm64" {
		add(doctorOK, "platform", platform)
	} else {
		add(doctorFail, "platform", platform+" is unsupported")
	}
	add(doctorOK, "lserve", "0.1.0 (built "+buildTime+")")

	info, err := os.Stat(h.Root())
	switch {
	case os.IsNotExist(err):
		add(doctorWarn, "state directory", h.Root()+" does not exist yet; run lserve up")
	case err != nil:
		add(doctorFail, "state directory", err.Error())
	case !info.IsDir():
		add(doctorFail, "state directory", h.Root()+" is not a directory")
	default:
		add(doctorOK, "state directory", h.Root())
	}

	checkBinary := func(name, path string, required bool) {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			status := doctorWarn
			detail := "not downloaded; installed on first use"
			if !required {
				status = doctorOK
				detail = "not installed (optional)"
			}
			add(status, name+" binary", detail)
			return
		}
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			add(doctorFail, name+" binary", path+" is not executable")
			return
		}
		add(doctorOK, name+" binary", path)
	}
	checkBinary("frankenphp", h.FrankenPHP(), true)
	checkBinary("mysqld", filepath.Join(h.Root(), "mysql", "bin", "mysqld"), true)
	checkBinary("mailpit", filepath.Join(h.Bin(), "mailpit"), false)

	if id, err := session.Current(); err != nil {
		add(doctorFail, "login session", err.Error())
	} else {
		add(doctorOK, "login session", id)
	}
	if ips, err := net.LookupIP("lserve-doctor.localhost"); err != nil || !loopbackOnly(ips) {
		add(doctorFail, "localhost DNS", "lserve-doctor.localhost does not resolve only to loopback")
	} else {
		add(doctorOK, "localhost DNS", "resolves to loopback")
	}

	franken := h.FrankenPHP()
	_, frankenErr := proc.Inspect(h.FrankenPHPpid(), franken)
	frankenRunning := frankenErr == nil
	httpsPort := portEnvInt("DEVENV_HTTPS_PORT", 8443)
	httpPort := portEnvInt("DEVENV_HTTP_PORT", 8080)
	addPort := func(name string, port int, owned bool) {
		check := diagnosePort(name, port, portAnswers(port), owned)
		report = append(report, check)
	}
	addPort("HTTPS port", httpsPort, frankenRunning)
	addPort("HTTP port", httpPort, false)
	if frankenRunning {
		add(doctorOK, "frankenphp", "running")
	} else {
		add(doctorWarn, "frankenphp", "stopped")
	}

	mysqlBin := filepath.Join(h.Bin(), "mysqld")
	_, mysqlErr := proc.Inspect(filepath.Join(h.Run(), "mysql", "mysqld.pid"), mysqlBin)
	mysqlRunning := mysqlErr == nil
	addPort("database port", portEnvInt("DEVENV_DB_PORT", 3306), mysqlRunning)
	mailpitBin := filepath.Join(h.Bin(), "mailpit")
	_, mailpitErr := proc.Inspect(filepath.Join(h.Run(), "mailpit.pid"), mailpitBin)
	mailpitRunning := mailpitErr == nil
	addPort("mailpit UI port", portEnvInt("DEVENV_MAILPIT_UI_PORT", 8025), mailpitRunning)
	addPort("mailpit SMTP port", portEnvInt("DEVENV_MAILPIT_SMTP_PORT", 1025), mailpitRunning)

	reg, err := registry.Open(h.Root())
	if err != nil {
		add(doctorFail, "registry", err.Error())
	} else {
		projects := map[string][]*site.Site{}
		for _, project := range reg.Projects {
			sites, err := site.DiscoverWithDocroot(project.Dir, project.DocRoot)
			if err != nil {
				add(doctorFail, "project", project.Dir+": "+err.Error())
				continue
			}
			projects[project.Dir] = sites
			for _, s := range sites {
				if s.Mode == site.ModeProxy && !portAnswersTarget(s.Target) {
					add(doctorWarn, "proxy upstream", s.Host()+" -> "+s.Target+" is not listening")
				}
			}
		}
		if len(reg.Projects) == 0 {
			add(doctorOK, "registry", "no projects registered")
		} else if _, err := caddy.Render(projects, httpPort, httpsPort); err != nil {
			add(doctorFail, "registry", err.Error())
		} else {
			add(doctorOK, "registry", fmt.Sprintf("%d project(s) valid", len(reg.Projects)))
		}
	}

	report.Write(os.Stdout)
	if report.Failed() {
		return fmt.Errorf("doctor found problems")
	}
	return nil
}

func diagnosePort(name string, port int, answering, owned bool) doctorCheck {
	check := doctorCheck{Name: name}
	switch {
	case answering && owned:
		check.Status = doctorOK
		check.Detail = fmt.Sprintf("%d in use by lserve", port)
	case answering:
		check.Status = doctorFail
		check.Detail = fmt.Sprintf("%d in use by another process", port)
	case owned:
		check.Status = doctorFail
		check.Detail = fmt.Sprintf("lserve is running but %d is not answering", port)
	default:
		check.Status = doctorOK
		check.Detail = fmt.Sprintf("%d free", port)
	}
	return check
}

func loopbackOnly(ips []net.IP) bool {
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}
