package types

// GroupIntegrations is the group-scoped integration configuration blob.
// Empty string fields fall back to the corresponding HBOX_* env value.
type GroupIntegrations struct {
	AIProvider                string `json:"aiProvider"` // "" inherit env | "disabled" force off | "openai_compatible" | "anthropic"
	AIBaseURL                 string `json:"aiBaseUrl"`
	AIAPIKey                  string `json:"aiApiKey"`
	AIModel                   string `json:"aiModel"`
	BarcodeTokenBarcodespider string `json:"barcodeTokenBarcodespider"`
	OpenFoodFactsContact      string `json:"openFoodFactsContact"`
}
