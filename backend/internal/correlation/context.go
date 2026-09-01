package correlation

import (
	"context"
	"github.com/gin-gonic/gin"
	"revenue-recovery/backend/internal/id"
)

type key struct{}

func With(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, key{}, value)
}
func From(ctx context.Context) string { value, _ := ctx.Value(key{}).(string); return value }
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		value := c.GetHeader("X-Correlation-ID")
		if value == "" {
			value = id.New()
		}
		c.Header("X-Correlation-ID", value)
		c.Request = c.Request.WithContext(With(c.Request.Context(), value))
		c.Next()
	}
}
