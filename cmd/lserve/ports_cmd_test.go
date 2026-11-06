package main

import (
	"testing"

	"github.com/EstebanForge/lite-serve/internal/ports"
)

func TestFormatHolder(t *testing.T) {
	tests := []struct {
		name   string
		holder ports.Holder
		want   string
	}{
		{
			name: "container plus forwarder pid",
			holder: ports.Holder{
				Port:    3306,
				PIDs:    []int{751782},
				Process: "rootlessport",
				Containers: []ports.Container{{
					Engine: "podman",
					ID:     "515e982b8474",
					Name:   "wicket-wp-stack_mariadb_1",
					Image:  "docker.io/library/mariadb:10.11",
				}},
			},
			want: "podman: wicket-wp-stack_mariadb_1 (docker.io/library/mariadb:10.11), pid 751782 rootlessport",
		},
		{
			name: "two containers",
			holder: ports.Holder{
				Port: 443,
				Containers: []ports.Container{
					{Engine: "podman", ID: "a", Name: "edge", Image: "caddy"},
					{Engine: "docker", ID: "b", Name: "web", Image: "nginx"},
				},
			},
			want: "podman: edge (caddy), docker: web (nginx)",
		},
		{
			name:   "container without pid",
			holder: ports.Holder{Port: 443, Containers: []ports.Container{{Engine: "docker", ID: "abc", Name: "web", Image: "nginx"}}},
			want:   "docker: web (nginx)",
		},
		{
			name:   "plain process",
			holder: ports.Holder{Port: 3306, PIDs: []int{955}, Process: "mysqld"},
			want:   "pid 955 mysqld",
		},
		{
			name:   "multiple listeners",
			holder: ports.Holder{Port: 8443, PIDs: []int{1, 2}, Process: "frankenphp, frankenphp"},
			want:   "pid 1 frankenphp, pid 2 frankenphp",
		},
		{
			name:   "anonymous holder",
			holder: ports.Holder{Port: 8080},
			want:   "unidentified process (no permission to inspect it)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatHolder(tt.holder); got != tt.want {
				t.Errorf("formatHolder = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWantsStop(t *testing.T) {
	for _, in := range []string{"y", "Y", "yes", "YES", " yes "} {
		if !wantsStop(in) {
			t.Errorf("wantsStop(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"n", "no", "", "later", "yeah-no"} {
		if wantsStop(in) {
			t.Errorf("wantsStop(%q) = true, want false", in)
		}
	}
}
