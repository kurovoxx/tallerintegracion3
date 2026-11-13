package service

// RateLimiter gestiona el límite de conversiones por usuario.
// Usa el rateTracker compartido definido en gemini_ai_service.go.
type RateLimiter struct {
	tracker *rateTracker
	limit   int
}

func NewRateLimiter(limit int) *RateLimiter {
	return &RateLimiter{
		tracker: newRateTracker(),
		limit:   limit,
	}
}

// Allow verifica si el usuario puede hacer una conversión.
func (rl *RateLimiter) Allow(userID string) bool {
	return rl.tracker.Allow(userID, rl.limit)
}

// Remaining devuelve cuántas conversiones le quedan al usuario en la hora actual.
func (rl *RateLimiter) Remaining(userID string) int {
	return rl.tracker.Remaining(userID, rl.limit)
}

// Limit devuelve el límite máximo de conversiones por hora.
func (rl *RateLimiter) Limit() int {
	return rl.limit
}
