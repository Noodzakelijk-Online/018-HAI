package main

import (
	"fmt"
	"os"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/memory"
	"automation-hub-backend/internal/reconcile"
	"automation-hub-backend/internal/router"
)

func main() {
	// Subcommands run a one-shot task and exit without starting the HTTP server.
	// Any other invocation starts the server exactly as before.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor":
			config.Init()
			os.Exit(doctor.Render(os.Stdout, doctor.Diagnose(config.AppConfig)))
		case "reconcile":
			os.Exit(runReconcile())
		}
	}

	config.Init()

	err := router.Initialize()
	if err != nil {
		panic(err)
	}
}

// runReconcile scans stored memories for broken invariants and prints a
// dry-run report. It requires a database connection and never mutates data.
func runReconcile() int {
	config.Init()
	db, err := infra.GetDefaultDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reconcile requires a database connection:", err)
		return 1
	}
	memories, err := memory.NewGormRepository(db).FindAll("", true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reconcile: failed to load memories:", err)
		return 1
	}
	report := reconcile.ScanMemories(memories)
	fmt.Printf("reconcile: scanned %d memories, %d finding(s)\n", report.Scanned, len(report.Findings))
	for _, f := range report.Findings {
		fmt.Printf("- %s repairable=%v: %s\n", f.MemoryID, f.Repairable, f.Repair)
	}
	if report.Clean() {
		fmt.Println("reconcile: all memories satisfy their invariants")
	}
	return 0
}
