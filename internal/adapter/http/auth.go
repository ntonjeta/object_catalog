package httpapi

import (
	"net/http"
	"time"

	"biblio-oggetto/internal/domain"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type userEnvelope struct {
	User userResponse `json:"user"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	user, token, expiresAt, err := s.auth.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	s.setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, userEnvelope{User: toUserResponse(user)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(sessionCookieName)
	if cookie != nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			writeError(w, err)
			return
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userEnvelope{User: toUserResponse(currentUser(r))})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var request passwordRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	user := currentUser(r)
	token, expiresAt, err := s.auth.ChangePassword(
		r.Context(), user.ID, request.CurrentPassword, request.NewPassword,
	)
	if err != nil {
		writeError(w, err)
		return
	}
	s.setSessionCookie(w, token, expiresAt)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if err := s.auth.DeleteAccount(r.Context(), user.ID); err != nil {
		writeError(w, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func toUserResponse(user domain.User) userResponse {
	return userResponse{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt}
}
