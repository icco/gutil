package otel

import (
	"fmt"

	"github.com/XSAM/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func InitPostgres() (string, error) {
	// Register an OTel driver
	driverName, err := otelsql.Register("postgres", semconv.DBSystemPostgreSQL.Value.AsString())
	if err != nil {
		return "", fmt.Errorf("could not create postgres otel: %w", err)
	}

	return driverName, nil
}
