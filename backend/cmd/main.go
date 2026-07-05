package main

import (
	"os"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/doctor"
	"automation-hub-backend/internal/router"
)

func main() {
	// `doctor` runs a self-diagnostic over the loaded configuration and exits,
	// without starting the HTTP server. Any other invocation starts the server
	// exactly as before.
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		config.Init()
		os.Exit(doctor.Render(os.Stdout, doctor.Diagnose(config.AppConfig)))
	}

	config.Init()

	err := router.Initialize()
	if err != nil {
		panic(err)
	}
}
