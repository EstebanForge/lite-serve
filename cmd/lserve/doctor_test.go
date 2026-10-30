package main

import (
	"strings"
	"testing"
)

func TestDoctorReportSeverity(t *testing.T) {
	report := doctorReport{
		{Status: doctorOK, Name: "platform", Detail: "linux-amd64"},
		{Status: doctorWarn, Name: "daemon", Detail: "stopped"},
	}
	if report.Failed() {
		t.Fatal("warnings must not fail doctor")
	}
	report = append(report, doctorCheck{Status: doctorFail, Name: "port", Detail: "occupied"})
	if !report.Failed() {
		t.Fatal("failure was not propagated")
	}
	var out strings.Builder
	report.Write(&out)
	for _, want := range []string{"OK   platform", "WARN daemon", "FAIL port"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}

func TestDoctorReportTreatsOptionalConflictAsWarning(t *testing.T) {
	check := diagnoseOptionalPort("Mailpit", 8025, true, false)
	report := doctorReport{check}
	if report.Failed() {
		t.Fatal("optional service conflict failed doctor")
	}
}

func TestPortOwnedByAnotherProcessFails(t *testing.T) {
	check := diagnosePort("HTTPS", 8443, true, false)
	if check.Status != doctorFail || !strings.Contains(check.Detail, "another process") {
		t.Fatalf("unexpected check: %+v", check)
	}
	check = diagnosePort("HTTPS", 8443, true, true)
	if check.Status != doctorOK {
		t.Fatalf("lserve-owned port was not healthy: %+v", check)
	}
	check = diagnosePort("HTTPS", 8443, false, true)
	if check.Status != doctorFail || !strings.Contains(check.Detail, "not answering") {
		t.Fatalf("unhealthy lserve port passed: %+v", check)
	}
}
