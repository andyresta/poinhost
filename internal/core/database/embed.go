package database

import "embed"

// embeddedMigrations menempel di package ini, BUKAN di package main, supaya
// setiap binary yang memakai schema poinhost — app desktop Wails maupun
// poinhost-agent nanti — memakai kumpulan migrasi yang sama persis tanpa perlu
// menyalin folder atau menyediakan embed.FS sendiri. Schema-nya satu, jadi
// sumbernya juga harus satu.
//
//go:embed migrations/*.sql
var embeddedMigrations embed.FS
