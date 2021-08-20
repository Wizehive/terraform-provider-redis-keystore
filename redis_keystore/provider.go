package redis_keystore

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"redis-keystore_keyset": resourceKeyset(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			// "redis_keyset": dataKeyset(),
		},
	}
}
