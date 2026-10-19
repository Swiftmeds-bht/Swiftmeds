package migrate

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadReadsOnlyUpFilesInVersionOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"002_add_index.up.sql":      {Data: []byte("CREATE INDEX i ON t(c);")},
		"001_create_table.up.sql":   {Data: []byte("CREATE TABLE t (c TEXT);")},
		"001_create_table.down.sql": {Data: []byte("DROP TABLE t;")},
		"migrations.go":             {Data: []byte("package migrations")},
		"README.md":                 {Data: []byte("notes")},
	}

	got, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}

	want := []Migration{
		{Version: "001_create_table", SQL: "CREATE TABLE t (c TEXT);"},
		{Version: "002_add_index", SQL: "CREATE INDEX i ON t(c);"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v\nwant %+v", got, want)
	}
}

func TestLoadEmptyDirectoryIsNotAnError(t *testing.T) {
	got, err := Load(fstest.MapFS{})
	if err != nil {
		t.Fatalf("Load(empty) error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Load(empty) = %v, want none", got)
	}
}

func TestLoadRejectsEmptyFile(t *testing.T) {
	fsys := fstest.MapFS{"001_blank.up.sql": {Data: []byte("  \n\t")}}

	_, err := Load(fsys)
	if err == nil || !strings.Contains(err.Error(), "001_blank.up.sql") {
		t.Fatalf("Load() error = %v, want error naming the empty file", err)
	}
}

func TestPending(t *testing.T) {
	all := []Migration{{Version: "001"}, {Version: "002"}, {Version: "003"}}

	cases := []struct {
		name string
		done map[string]bool
		want []string
	}{
		{"fresh database", map[string]bool{}, []string{"001", "002", "003"}},
		{"partly migrated", map[string]bool{"001": true}, []string{"002", "003"}},
		{"fully migrated", map[string]bool{"001": true, "002": true, "003": true}, nil},
		{"gap is still applied", map[string]bool{"001": true, "003": true}, []string{"002"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, m := range Pending(all, tc.done) {
				got = append(got, m.Version)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Pending() = %v, want %v", got, tc.want)
			}
		})
	}
}
