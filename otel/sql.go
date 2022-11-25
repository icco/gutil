package otel

import (
	"fmt"

	"github.com/XSAM/otelsql"
)

func InitPostgres() (string, error) {
	// Register an OTel driver
	driverName, err := otelsql.Register("postgres")
	if err != nil {
		return "", fmt.Errorf("could not create postgres otel: %w", err)
	}

	return driverName, nil
}
