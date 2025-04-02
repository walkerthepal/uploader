package middleware

import (
	"github.com/go-chi/chi/v5/middleware"
)

// Logger is a middleware that logs HTTP requests
var Logger = middleware.Logger
