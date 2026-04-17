package db

import "testing"

func TestExtractEntraAuth(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantOK   bool
		wantOut  string
	}{
		{
			name:    "url with entra flag",
			in:      "postgres://alice@db.postgres.database.azure.com:5432/rstash?sslmode=require&auth=entra",
			wantOK:  true,
			wantOut: "postgres://alice@db.postgres.database.azure.com:5432/rstash?sslmode=require",
		},
		{
			name:    "postgresql scheme also recognized",
			in:      "postgresql://svc@host/rstash?auth=entra&sslmode=require",
			wantOK:  true,
			wantOut: "postgresql://svc@host/rstash?sslmode=require",
		},
		{
			name:    "entra case-insensitive",
			in:      "postgres://u@h/db?auth=Entra",
			wantOK:  true,
			wantOut: "postgres://u@h/db",
		},
		{
			name:    "no auth param",
			in:      "postgres://user:pw@localhost/rstash?sslmode=disable",
			wantOK:  false,
			wantOut: "postgres://user:pw@localhost/rstash?sslmode=disable",
		},
		{
			name:    "auth param set to something else",
			in:      "postgres://u@h/db?auth=password",
			wantOK:  false,
			wantOut: "postgres://u@h/db?auth=password",
		},
		{
			name:    "keyword-value form (not supported, passes through)",
			in:      "host=localhost user=postgres dbname=rstash",
			wantOK:  false,
			wantOut: "host=localhost user=postgres dbname=rstash",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractEntraAuth(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.wantOut {
				t.Errorf("out = %q, want %q", got, tc.wantOut)
			}
		})
	}
}
