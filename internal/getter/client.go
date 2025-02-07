package getter

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

type Client struct {
	dynamic   dynamic.Interface
	discovery discovery.CachedDiscoveryInterface
}

func NewClient(dynamic dynamic.Interface, discovery discovery.CachedDiscoveryInterface) *Client {
	return &Client{
		dynamic:   dynamic,
		discovery: discovery,
	}
}
