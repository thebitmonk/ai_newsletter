package httpx

import "github.com/gin-gonic/gin"

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: message},
	})
}

func ErrorWithDetails(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: message, Details: details},
	})
}
