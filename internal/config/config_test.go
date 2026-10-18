package config

import (
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		env        map[string]string
		wantDir    string
		wantAddr   string
		wantHealth bool
		wantErr    bool
	}{
		{name: "defaults", wantDir: "data", wantAddr: ":8080"},
		{
			name:     "environment",
			env:      map[string]string{"GOTREE_DATA_DIR": "/srv/tree", "GOTREE_LISTEN_ADDR": "127.0.0.1:9000"},
			wantDir:  "/srv/tree",
			wantAddr: "127.0.0.1:9000",
		},
		{
			name:     "flags win over environment",
			args:     []string{"-data-dir", "/flag", "-listen", ":1"},
			env:      map[string]string{"GOTREE_DATA_DIR": "/env", "GOTREE_LISTEN_ADDR": ":2"},
			wantDir:  "/flag",
			wantAddr: ":1",
		},
		{name: "healthcheck", args: []string{"-healthcheck"}, wantDir: "data", wantAddr: ":8080", wantHealth: true},
		{name: "empty data dir", args: []string{"-data-dir", ""}, wantErr: true},
		{name: "unknown flag", args: []string{"-nope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.args, env(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DataDir != tt.wantDir || cfg.ListenAddr != tt.wantAddr || cfg.HealthCheck != tt.wantHealth {
				t.Errorf("got %+v, want dir %q addr %q healthcheck %v", cfg, tt.wantDir, tt.wantAddr, tt.wantHealth)
			}
		})
	}
}

func TestDBPath(t *testing.T) {
	cfg := Config{DataDir: "/x"}
	if got, want := cfg.DBPath(), filepath.Join("/x", "gotree.db"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
