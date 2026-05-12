package handlers

import (
	"github.com/gofiber/fiber/v2"
)

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
}

func CustomErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	errorCode := "internal_error"
	message := "Internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		switch code {
		case 404:
			errorCode = "transaction_not_found"
			message = e.Message
		case 400:
			errorCode = "validation_failed"
			message = e.Message
		case 409:
			errorCode = "incident_already_resolved"
			message = e.Message
		}
	}

	return c.Status(code).JSON(ErrorResponse{
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details any    `json:"details,omitempty"`
		}{
			Code:    errorCode,
			Message: message,
		},
	})
}