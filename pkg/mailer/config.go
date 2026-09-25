package mailer

type Config struct {
	ResendAPIKey    string
	ResendFromEmail string
	BrevoAPIKey     string
	BrevoFromEmail  string
	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPass        string
	FromEmail       string
}
