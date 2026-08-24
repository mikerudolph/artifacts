package api

import "github.com/go-chi/chi/v5"

var (
	active        *server
	contentRoutes func(chi.Router)
	jobRoutes     func(chi.Router)
)

func registerContent(r chi.Router) {
	if contentRoutes != nil {
		contentRoutes(r)
	}
}

func registerJobs(r chi.Router) {
	if jobRoutes != nil {
		jobRoutes(r)
	}
}
