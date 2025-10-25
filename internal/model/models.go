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
}
