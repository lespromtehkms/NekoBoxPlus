package option

import "github.com/sagernet/sing/common/json/badoption"

type YggdrasilEndpointOptions struct {
	DialerOptions
	PrivateKey        string                     `json:"private_key,omitempty"`
	Peers             badoption.Listable[string] `json:"peers,omitempty"`
	AllowedPublicKeys badoption.Listable[string] `json:"allowed_public_keys,omitempty"`
	GroupPassword     string                     `json:"group_password,omitempty"`
	MTU               uint32                     `json:"mtu,omitempty"`
}
