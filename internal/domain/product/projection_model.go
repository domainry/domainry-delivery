package product

type ProductProjection struct {
	Product          Product           `json:"product"`
	AvailableActions []AvailableAction `json:"available_actions"`
}

type ProductAgentContext struct {
	Product              Product           `json:"product"`
	AllowedAgentCommands []string          `json:"allowed_agent_commands"`
	AvailableActions     []AvailableAction `json:"available_actions"`
	HumanOnlyCommands    []string          `json:"human_only_commands"`
	SystemOnlyCommands   []string          `json:"system_only_commands"`
	SourcePolicy         string            `json:"source_policy"`
}
