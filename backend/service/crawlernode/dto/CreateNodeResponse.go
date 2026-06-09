package dto

// CreateNodeResponse is returned exactly once when a node is provisioned.
// The Token field contains the plaintext bearer token — never stored and
// never returned again. The caller must save it immediately.
type CreateNodeResponse struct {
	NodeView
	Token string `json:"token"`
}
