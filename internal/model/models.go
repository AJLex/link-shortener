package models

const (
	TypeSimpleUtterance = "SimpleUtterance"
	TypeApplicationJSON = "application/json"
	TypeTextPlain       = "text/plain"
)

// ShortenRequest модель для десериализации входящего запроса
type ShortenRequest struct {
	URL string `json:"url"`
}

// ShortenResponse модель для сериализации исходящего ответа
type ShortenResponse struct {
	Result string `json:"result"`
}

type URLEntry struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
	UserID      string `json:"user_id,omitempty"`
}

// BatchRequestItem элемент запроса на пакетное сокращение
type BatchRequestItem struct {
	CorrelationID string `json:"correlation_id"`
	OriginalURL   string `json:"original_url"`
}

// BatchResponseItem элемент ответа на пакетное сокращение
type BatchResponseItem struct {
	CorrelationID string `json:"correlation_id"`
	ShortURL      string `json:"short_url"`
}

// UserURL модель для возврата URL пользователя
type UserURL struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}
