package config

import (
	"path/filepath"
	"slices"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		wantDir  string
		wantAddr string
		wantErr  bool
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
			if cfg.DataDir != tt.wantDir || cfg.ListenAddr != tt.wantAddr {
				t.Errorf("got %+v, want dir %q addr %q", cfg, tt.wantDir, tt.wantAddr)
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

func TestPhotoLocation(t *testing.T) {
	for value, want := range map[string]bool{"": false, "strip": false, "keep": true} {
		cfg, err := Load(nil, env(map[string]string{"GOTREE_PHOTO_LOCATION": value}))
		if err != nil || cfg.KeepPhotoLocation != want {
			t.Errorf("%q: keep %v, err %v; want keep %v", value, cfg.KeepPhotoLocation, err, want)
		}
	}
	if _, err := Load(nil, env(map[string]string{"GOTREE_PHOTO_LOCATION": "foo"})); err == nil {
		t.Error("foo accepted")
	}
}

func TestTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{name: "list", value: "127.0.0.1, 10.0.0.0/8 ,::1", want: []string{"127.0.0.1/32", "10.0.0.0/8", "::1/128"}},
		{name: "unset"},
		{name: "masked", value: "10.0.0.1/8", want: []string{"10.0.0.0/8"}},
		{name: "not an address", value: "proxy.local", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(nil, env(map[string]string{"GOTREE_TRUSTED_PROXIES": tt.value}))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			var got []string
			for _, p := range cfg.TrustedProxies {
				got = append(got, p.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("TrustedProxies = %v, want %v", got, tt.want)
			}
		})
	}
}
