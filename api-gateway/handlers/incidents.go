package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"yourproject/api/db"
	"yourproject/api/models"
)

func GetIncidents(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	rows, err := db.Pool.Query(c.Context(),
		`SELECT id, transaction_id, incident_type, severity, description, status, created_at,
			acknowledged_at, resolved_at, auto_correction_proposed
		FROM incidents
		WHERE status = 'open'
		ORDER BY severity DESC, created_at ASC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	incidents := []models.Incident{}
	for rows.Next() {
		var inc models.Incident
		var ackAt, resAt *time.Time
		var autoCorr []byte
		err := rows.Scan(&inc.ID, &inc.TransactionID, &inc.IncidentType, &inc.Severity, &inc.Description,
			&inc.Status, &inc.CreatedAt, &ackAt, &resAt, &autoCorr)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		inc.AcknowledgedAt = ackAt
		inc.ResolvedAt = resAt
		if len(autoCorr) > 0 {
			inc.AutoCorrectionProposed = autoCorr
		}
		inc.AffectedSources = []string{}
		incidents = append(incidents, inc)
	}
	return c.JSON(incidents)
}

func AcknowledgeIncident(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid incident id"})
	}

	now := time.Now()
	_, err = db.Pool.Exec(c.Context(),
		`UPDATE incidents SET status = 'acknowledged', acknowledged_at = $1 WHERE id = $2 AND status = 'open'`,
		now, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "acknowledged"})
}