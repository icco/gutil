package otel

import (
	"database/sql"
	"fmt"

	"github.com/XSAM/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func InitPostgres() (*sql.Driver, error) {
	// Register an OTel driver
	driverName, err := otelsql.Register("postgres", semconv.DBSystemPostgreSQL.Value)
	if err != nil {
		return nil, fmt.Errorf("could not create postgres otel: %w", err)
	}

	return driverName, nil
}
