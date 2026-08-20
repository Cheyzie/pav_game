package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/Cheyzie/pav_game/internal/model"
	"github.com/Cheyzie/pav_game/internal/service"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	authService *service.AuthorizationService
	gameService *service.GameService
	upgrader    websocket.Upgrader
}

func (h *Handler) options(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.WriteHeader(http.StatusNoContent)
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewHandler(authService *service.AuthorizationService, gameService *service.GameService) *Handler {
	return &Handler{
		authService: authService,
		gameService: gameService,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Пропускаем любой запрос
			},
		},
	}
}

func (h *Handler) InitRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		newResponse(w, http.StatusOK, map[string]any{
			"message": "hello world",
		})
	})
	{
		mux.HandleFunc("OPTIONS /api/v1/signin", h.options)
		mux.HandleFunc("POST /api/v1/signin", corsMiddleware(h.signIn))
		mux.HandleFunc("OPTIONS /api/v1/signup", h.options)
		mux.HandleFunc("POST /api/v1/signup", corsMiddleware(h.signUp))
		mux.HandleFunc("OPTIONS /api/v1/refresh", h.options)
		mux.HandleFunc("POST /api/v1/refresh", corsMiddleware(h.refreshToken))
		mux.HandleFunc("OPTIONS /api/v1/me", h.options)
		mux.HandleFunc("GET /api/v1/me", corsMiddleware(h.userIdentityMiddleware(h.getMe)))
		mux.HandleFunc("OPTIONS /api/v1/me/sessions", h.options)
		mux.HandleFunc("GET /api/v1/me/sessions", corsMiddleware(h.userIdentityMiddleware(h.userSessions)))
		mux.HandleFunc("OPTIONS /api/v1/sessions/:id", h.options)
		mux.HandleFunc("DELETE /api/v1/me/sessions/:id", corsMiddleware(h.userIdentityMiddleware(h.dropSession)))
		mux.HandleFunc("OPTIONS /api/v1/logout", h.options)
		mux.HandleFunc("DELETE /api/v1/logout", corsMiddleware(h.userIdentityMiddleware(h.logout)))

		mux.HandleFunc("OPTIONS /api/v1/rooms", h.options)
		mux.HandleFunc("POST /api/v1/rooms", corsMiddleware(h.userIdentityMiddleware(h.createRoom)))
		mux.HandleFunc("OPTIONS /api/v1/rooms/{code}/join", h.options)
		mux.HandleFunc("POST /api/v1/rooms/{code}/join", corsMiddleware(h.userIdentityMiddleware(h.join)))
		mux.HandleFunc("OPTIONS /api/v1/rooms/connect", h.options)
		mux.HandleFunc("GET /api/v1/rooms/connect", corsMiddleware(h.connect))
	}

	return mux
}

func newErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	if err != nil {
		logrus.Error(err.Error())
	}
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(model.Error{Message: message}); err != nil {
		logrus.Error(err.Error())
	}
}

func newResponse(w http.ResponseWriter, statusCode int, body any) {
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(body); err != nil {
		logrus.Error(err.Error())
	}
}
