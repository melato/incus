//go:build linux && cgo && !agent

package cluster

// The code below was generated by incus-generate - DO NOT EDIT!

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lxc/incus/incus/db/query"
	"github.com/lxc/incus/shared/api"
)

var _ = api.ServerEnvironment{}

var certificateProjectObjects = RegisterStmt(`
SELECT certificates_projects.certificate_id, certificates_projects.project_id
  FROM certificates_projects
  ORDER BY certificates_projects.certificate_id
`)

var certificateProjectObjectsByCertificateID = RegisterStmt(`
SELECT certificates_projects.certificate_id, certificates_projects.project_id
  FROM certificates_projects
  WHERE ( certificates_projects.certificate_id = ? )
  ORDER BY certificates_projects.certificate_id
`)

var certificateProjectCreate = RegisterStmt(`
INSERT INTO certificates_projects (certificate_id, project_id)
  VALUES (?, ?)
`)

var certificateProjectDeleteByCertificateID = RegisterStmt(`
DELETE FROM certificates_projects WHERE certificate_id = ?
`)

// certificateProjectColumns returns a string of column names to be used with a SELECT statement for the entity.
// Use this function when building statements to retrieve database entries matching the CertificateProject entity.
func certificateProjectColumns() string {
	return "certificates_projects.certificate_id, certificates_projects.project_id"
}

// getCertificateProjects can be used to run handwritten sql.Stmts to return a slice of objects.
func getCertificateProjects(ctx context.Context, stmt *sql.Stmt, args ...any) ([]CertificateProject, error) {
	objects := make([]CertificateProject, 0)

	dest := func(scan func(dest ...any) error) error {
		c := CertificateProject{}
		err := scan(&c.CertificateID, &c.ProjectID)
		if err != nil {
			return err
		}

		objects = append(objects, c)

		return nil
	}

	err := query.SelectObjects(ctx, stmt, dest, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"certificates_projects\" table: %w", err)
	}

	return objects, nil
}

// getCertificateProjectsRaw can be used to run handwritten query strings to return a slice of objects.
func getCertificateProjectsRaw(ctx context.Context, tx *sql.Tx, sql string, args ...any) ([]CertificateProject, error) {
	objects := make([]CertificateProject, 0)

	dest := func(scan func(dest ...any) error) error {
		c := CertificateProject{}
		err := scan(&c.CertificateID, &c.ProjectID)
		if err != nil {
			return err
		}

		objects = append(objects, c)

		return nil
	}

	err := query.Scan(ctx, tx, sql, dest, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"certificates_projects\" table: %w", err)
	}

	return objects, nil
}

// GetCertificateProjects returns all available Projects for the Certificate.
// generator: certificate_project GetMany
func GetCertificateProjects(ctx context.Context, tx *sql.Tx, certificateID int) ([]Project, error) {
	var err error

	// Result slice.
	objects := make([]CertificateProject, 0)

	sqlStmt, err := Stmt(tx, certificateProjectObjectsByCertificateID)
	if err != nil {
		return nil, fmt.Errorf("Failed to get \"certificateProjectObjectsByCertificateID\" prepared statement: %w", err)
	}

	args := []any{certificateID}

	// Select.
	objects, err = getCertificateProjects(ctx, sqlStmt, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch from \"certificates_projects\" table: %w", err)
	}

	result := make([]Project, len(objects))
	for i, object := range objects {
		project, err := GetProjects(ctx, tx, ProjectFilter{ID: &object.ProjectID})
		if err != nil {
			return nil, err
		}

		result[i] = project[0]
	}

	return result, nil
}

// DeleteCertificateProjects deletes the certificate_project matching the given key parameters.
// generator: certificate_project DeleteMany
func DeleteCertificateProjects(ctx context.Context, tx *sql.Tx, certificateID int) error {
	stmt, err := Stmt(tx, certificateProjectDeleteByCertificateID)
	if err != nil {
		return fmt.Errorf("Failed to get \"certificateProjectDeleteByCertificateID\" prepared statement: %w", err)
	}

	result, err := stmt.Exec(int(certificateID))
	if err != nil {
		return fmt.Errorf("Delete \"certificates_projects\" entry failed: %w", err)
	}

	_, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("Fetch affected rows: %w", err)
	}

	return nil
}

// CreateCertificateProjects adds a new certificate_project to the database.
// generator: certificate_project Create
func CreateCertificateProjects(ctx context.Context, tx *sql.Tx, objects []CertificateProject) error {
	for _, object := range objects {
		args := make([]any, 2)

		// Populate the statement arguments.
		args[0] = object.CertificateID
		args[1] = object.ProjectID

		// Prepared statement to use.
		stmt, err := Stmt(tx, certificateProjectCreate)
		if err != nil {
			return fmt.Errorf("Failed to get \"certificateProjectCreate\" prepared statement: %w", err)
		}

		// Execute the statement.
		_, err = stmt.Exec(args...)
		if err != nil {
			return fmt.Errorf("Failed to create \"certificates_projects\" entry: %w", err)
		}

	}

	return nil
}

// UpdateCertificateProjects updates the certificate_project matching the given key parameters.
// generator: certificate_project Update
func UpdateCertificateProjects(ctx context.Context, tx *sql.Tx, certificateID int, projectNames []string) error {
	// Delete current entry.
	err := DeleteCertificateProjects(ctx, tx, certificateID)
	if err != nil {
		return err
	}

	// Get new entry IDs.
	certificateProjects := make([]CertificateProject, 0, len(projectNames))
	for _, entry := range projectNames {
		refID, err := GetProjectID(ctx, tx, entry)
		if err != nil {
			return err
		}

		certificateProjects = append(certificateProjects, CertificateProject{CertificateID: certificateID, ProjectID: int(refID)})
	}

	err = CreateCertificateProjects(ctx, tx, certificateProjects)
	if err != nil {
		return err
	}

	return nil
}
