package handlers

import (
	"github.com/gofiber/fiber/v2"
	"yourproject/api/db"
	"yourproject/api/models"
)

func MismatchPerMinute(c *fiber.Ctx) error {
	hours := c.QueryInt("hours", 1)
	query := `
		SELECT date_trunc('minute', created_at) as minute, COUNT(*) 
		FROM incidents 
		WHERE incident_type IN ('amount_mismatch', 'fee_mismatch') 
		  AND created_at > NOW() - make_interval(hours => $1)
		GROUP BY minute
		ORDER BY minute ASC
	`
	rows, err := db.Pool.Query(c.Context(), query, hours)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	results := make([]models.MismatchPerMinute, 0)
	for rows.Next() {
		var mm models.MismatchPerMinute
		err := rows.Scan(&mm.Minute, &mm.MismatchCount)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		results = append(results, mm)
	}
	return c.JSON(results)
}