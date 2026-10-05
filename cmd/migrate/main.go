// Command migrate applies the embedded PostgreSQL migrations outside the
// app's boot path: CI gates run `migrate up` against a staging database,
// operators verify a deploy's migration set with `migrate version`, and a
// failed mid-flight migration is recovered with `migrate force`.
//
// The binary reads APP_DATABASE_URL (or APP_DATABASE_URL_FILE, the mounted
// secret pattern) and walks the migrations directory on disk — the same
// directory the app embeds at boot, so both paths always ship one schema.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/your-org/go-template/internal/platform/postgres"
)

const migrationsDir = "migrations/postgres"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	url := databaseURL()
	if url == "" {
		fmt.Fprintln(os.Stderr, "migrate: APP_DATABASE_URL (or APP_DATABASE_URL_FILE) is required")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "up":
		if err := postgres.RunMigrations(url, os.DirFS(migrationsDir)); err != nil {
			fail(err)
		}
		version, dirty, err := postgres.MigrationVersion(url, os.DirFS(migrationsDir))
		if err != nil {
			fail(err)
		}
		fmt.Printf("migrations applied; schema version %d (dirty: %v)\n", version, dirty)
	case "version":
		version, dirty, err := postgres.MigrationVersion(url, os.DirFS(migrationsDir))
		if err != nil {
			fail(err)
		}
		fmt.Printf("schema version %d (dirty: %v)\n", version, dirty)
	case "force":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "migrate force: a version number is required (use -1 to clear a dirty state)")
			os.Exit(2)
		}
		version, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fail(fmt.Errorf("force: %w", err))
		}
		if err := postgres.ForceVersion(url, os.DirFS(migrationsDir), version); err != nil {
			fail(err)
		}
		fmt.Printf("forced schema version to %d\n", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate up | migrate version | migrate force <version>")
}

// databaseURL resolves the connection string, honoring the mounted-file
// secret pattern the app itself uses.
func databaseURL() string {
	if file := os.Getenv("APP_DATABASE_URL_FILE"); file != "" {
		// #nosec G304,G703 -- the operator-provided secret mount (docker
		// secrets pattern); reading it is the feature.
		data, err := os.ReadFile(file)
		if err != nil {
			fail(err)
		}
		return strings.TrimSpace(string(data))
	}
	return os.Getenv("APP_DATABASE_URL")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "migrate:", err)
	os.Exit(1)
}
