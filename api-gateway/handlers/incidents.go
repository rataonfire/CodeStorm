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
	if limit > 200 {
		limit = 200
	}
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	status := c.Query("status", "open")
	severity := c.Query("severity")

	query := `
		SELECT id, transaction_id, incident_type, severity, description, status, created_at,
			acknowledged_at, resolved_at, auto_correction_proposed
		FROM incidents
		WHERE status = $1
	`
	args := []interface{}{status}
	argIdx := 2

	if severity != "" {
		query += " AND severity = $" + strconv.Itoa(argIdx)
		args = append(args, severity)
		argIdx++
	}

	query += " ORDER BY severity DESC, created_at ASC"
	query += " LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := db.Pool.Query(c.Context(), query, args...)
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
	return c.JSON(fiber.Map{
		"items":          incidents,
		"total_estimate": len(incidents),
		"next_cursor":    nil,
	})
}

func AcknowledgeIncident(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fiber.Map{"code": "validation_failed", "message": "invalid incident id"}})
	}

	now := time.Now()
	tag, err := db.Pool.Exec(c.Context(),
		`UPDATE incidents SET status = 'acknowledged', acknowledged_at = $1 WHERE id = $2 AND status = 'open'`,
		now, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(409).JSON(fiber.Map{"error": fiber.Map{
			"code":    "incident_already_resolved",
			"message": "incident not found or already closed",
		}})
	}

	// Cancel escalation from Redis so it doesn't escalate after ack
	if redisClient != nil {
		redisClient.ZRem(c.Context(), "incident_escalations", id)
	}

	return c.JSON(fiber.Map{"status": "acknowledged"})
}

func ResolveIncident(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fiber.Map{"code": "validation_failed", "message": "invalid incident id"}})
	}

	now := time.Now()
	tag, err := db.Pool.Exec(c.Context(),
		`UPDATE incidents SET status = 'resolved', resolved_at = $1 WHERE id = $2 AND status != 'resolved'`,
		now, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if tag.RowsAffected() == 0 {
		return c.Status(409).JSON(fiber.Map{"error": fiber.Map{
			"code":    "incident_already_resolved",
			"message": "incident not found or already resolved",
		}})
	}

	if redisClient != nil {
		redisClient.ZRem(c.Context(), "incident_escalations", id)
	}

	return c.JSON(fiber.Map{"status": "resolved"})
}
