package website

import "testing"

func TestDBEngineBinary(t *testing.T) {
	cases := []struct {
		engine  string
		dump    string
		restore string
		wantOK  bool
	}{
		{"mysql", "mysqldump", "mysql", true},
		{"postgresql", "pg_dump", "psql", true},
		{"oracle", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		dump, restore, ok := DBEngineBinary(c.engine)
		if ok != c.wantOK || dump != c.dump || restore != c.restore {
			t.Errorf("DBEngineBinary(%q) = (%q, %q, %v), mau (%q, %q, %v)",
				c.engine, dump, restore, ok, c.dump, c.restore, c.wantOK)
		}
	}
}
