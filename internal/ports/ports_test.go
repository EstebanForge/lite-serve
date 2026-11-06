package ports

import (
	"reflect"
	"testing"
	"time"
)

func TestParseSsListeners(t *testing.T) {
	tests := []struct {
		name  string
		port  int
		out   string
		pids  []int
		names string
	}{
		{
			name:  "wildcard with pid",
			port:  3306,
			out:   `LISTEN 0      4096                                        *:3306        *:* users:(("rootlessport",pid=751782,fd=11))`,
			pids:  []int{751782},
			names: "rootlessport",
		},
		{
			name: "loopback and ipv6 duplicates merge",
			port: 8443,
			out: `LISTEN 0      4096                                127.0.0.1:8443  0.0.0.0:* users:(("frankenphp",pid=1741730,fd=8))
LISTEN 0      4096                                    [::1]:8443     [::]:* users:(("frankenphp",pid=1741730,fd=10))
LISTEN 0      4096                                127.0.0.1:8443  0.0.0.0:* users:(("frankenphp",pid=137629,fd=4))`,
			pids:  []int{1741730, 137629},
			names: "frankenphp, frankenphp",
		},
		{
			name:  "no process field stays parseable",
			port:  80,
			out:   "LISTEN 0      128                                        *:80          *:*",
			pids:  nil,
			names: "",
		},
		{
			name:  "other ports ignored",
			port:  80,
			out:   `LISTEN 0      128                                 0.0.0.0:8080          *:* users:(("nc",pid=1,fd=3))`,
			pids:  nil,
			names: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSs(tt.out, tt.port)
			if !reflect.DeepEqual(got.pids, tt.pids) {
				t.Errorf("pids = %v, want %v", got.pids, tt.pids)
			}
			if got.names != tt.names {
				t.Errorf("names = %q, want %q", got.names, tt.names)
			}
		})
	}
}

func TestParseLsofListeners(t *testing.T) {
	tests := []struct {
		name  string
		port  int
		out   string
		pids  []int
		names string
	}{
		{
			name:  "standard listen line",
			port:  8443,
			out:   "frankenphp 1741730 esteban    8u  IPv4  0xd0f1e2      0t0  TCP *:8443 (LISTEN)",
			pids:  []int{1741730},
			names: "frankenphp",
		},
		{
			name: "non-listen and other-port lines ignored",
			port: 3306,
			out: `mysqld 955 esteban   30u  IPv6  0xd0a1b2      0t0  TCP 127.0.0.1:3306 (LISTEN)
curl 401 esteban    5u  IPv4  0xc0a1b2      0t0  TCP 127.0.0.1:55123->127.0.0.1:3306 (ESTABLISHED)`,
			pids:  []int{955},
			names: "mysqld",
		},
		{
			name:  "space-named process shifts fields and drops the line",
			port:  3306,
			out:   "my app 955 esteban   30u  IPv6  0xd0a1b2      0t0  TCP 127.0.0.1:3306 (LISTEN)\nmysqld 955 esteban   31u  IPv6  0xd0a1b3      0t0  TCP [::1]:3306 (LISTEN)",
			pids:  []int{955},
			names: "mysqld",
		},
		{
			name:  "empty output",
			port:  8025,
			out:   "",
			pids:  nil,
			names: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLsof(tt.out, tt.port)
			if !reflect.DeepEqual(got.pids, tt.pids) {
				t.Errorf("pids = %v, want %v", got.pids, tt.pids)
			}
			if got.names != tt.names {
				t.Errorf("names = %q, want %q", got.names, tt.names)
			}
		})
	}
}

func TestParsePsPorts(t *testing.T) {
	tests := []struct {
		name    string
		fields  string
		host    int
		publish bool
	}{
		{name: "wildcard ipv4 match", fields: "0.0.0.0:3306->3306/tcp", host: 3306, publish: true},
		{name: "loopback match", fields: "127.0.0.1:3306->3306/tcp", host: 3306, publish: true},
		{name: "ipv6 match", fields: "[::]:3306->3306/tcp", host: 3306, publish: true},
		{
			name:    "multiple mappings second matches",
			fields:  "0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp",
			host:    443,
			publish: true,
		},
		{name: "exposed only never matches", fields: "9000/tcp", host: 9000, publish: false},
		{name: "empty field", fields: "", host: 3306, publish: false},
		{name: "different port", fields: "0.0.0.0:3306->3306/tcp", host: 3307, publish: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publishes(tt.fields, tt.host); got != tt.publish {
				t.Errorf("publishes(%q, %d) = %v, want %v", tt.fields, tt.host, got, tt.publish)
			}
		})
	}
}

func TestParsePsTable(t *testing.T) {
	out := "c8a4c7d3c202\twicket-wp-stack_php_1\tlocalhost/wicket-wp-stack_php:latest\t9000/tcp\n" +
		"515e982b8474\twicket-wp-stack_mariadb_1\tdocker.io/library/mariadb:10.11\t0.0.0.0:3306->3306/tcp\n" +
		"e38b656f1147\twicket-wp-stack_cron_1\tlocalhost/wicket-wp-stack_cron:latest\t\n" +
		"garbage line without tabs\n"
	got := parsePs(out, "podman", 3306)
	want := []Container{{
		Engine: "podman",
		ID:     "515e982b8474",
		Name:   "wicket-wp-stack_mariadb_1",
		Image:  "docker.io/library/mariadb:10.11",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePs = %+v, want %+v", got, want)
	}
}

func TestMergeContainersDedupesByID(t *testing.T) {
	a := Container{Engine: "docker", ID: "515e982b8474", Name: "mariadb_1", Image: "mariadb:10.11"}
	b := Container{Engine: "podman", ID: "515e982b8474", Name: "mariadb_1", Image: "mariadb:10.11"}
	got := mergeContainers([]Container{a}, []Container{b})
	if len(got) != 1 || got[0].Engine != "docker" {
		t.Errorf("mergeContainers = %+v, want single docker entry", got)
	}
}

func TestWaitFreeImmediate(t *testing.T) {
	// Nothing listens on this port in the test process; the probe must
	// return without waiting out the timeout.
	if !WaitFree(10944, 50*time.Millisecond) {
		t.Fatal("WaitFree reported a port free of listeners as held")
	}
}
