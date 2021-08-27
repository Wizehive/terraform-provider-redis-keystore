package redis_keystore

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/mediocregopher/radix/v4"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/r3labs/diff/v2"
)

func resourceKeyset() *schema.Resource {
	return &schema.Resource{
		Description:   "`redis_keyset` manages a list of key/val pairs in redis.",
		CreateContext: resourceKeysetCreate,
		ReadContext:   resourceKeysetRead,
		UpdateContext: resourceKeysetUpdate,
		DeleteContext: resourceKeysetDelete,
		Schema: map[string]*schema.Schema{
			"keyset": {
				Description: "The keys and values of the Keyset.",
				Type:        schema.TypeMap,
				Required:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"hostname": {
				Description: "Server hostname. Can be specified with the `REDISDB_HOSTNAME` environment variable. Defaults to `127.0.0.1`",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("REDISDB_HOSTNAME", "127.0.0.1"),
			},
			"port": {
				Description: "Server port. Can be specified with the `REDISDB_PORT` environment variable. Defaults to `6379`",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("REDISDB_PORT", "6379"),
			},
			"database": {
				Description: "Database number. Can be specified with the `REDISDB_DATABASE` environment variable. Defaults to `0`",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("REDISDB_DATABASE", "0"),
			},
			"bastion_host": {
				Description: "Host to use as a bastion tunnel. Can be specified with the `BASTION_HOST` environment variable.",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("BASTION_HOST", "nil"),
			},
			"bastion_user": {
				Description: "User to connect to the bastion as. Can be specified with the `BASTION_USER` environment variable.",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("BASTION_USER", ""),
			},
			"bastion_private_key": {
				Description: "File containing the bastion private key. Can be specified with the `BASTION_PRIVATE_KEY` environment variable.",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("BASTION_PRIVATE_KEY", ""),
			},
		},
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

func getRedisConnection(ctx context.Context, d *schema.ResourceData) (radix.Client, error) {
	// SSH tunnel code from: https://elliotchance.medium.com/how-to-create-an-ssh-tunnel-in-go-b63722d682aa

	hostname := d.Get("hostname").(string)
	port := d.Get("port").(string)
	database := d.Get("database").(string)
	bastion_host := d.Get("bastion_host").(string)
	bastion_user := d.Get("bastion_user").(string)
	bastion_private_key := d.Get("bastion_private_key").(string)

	cfg := radix.PoolConfig{
		Dialer: radix.Dialer{
			SelectDB: database,
		},
	}

	if bastion_host != "" && bastion_user != "" && bastion_private_key != "" {
		tunnel := NewSSHTunnel(
			fmt.Sprintf("%s@%s", bastion_user, bastion_host),
			PrivateKey([]byte(bastion_private_key)),
			fmt.Sprintf("%s:%s", hostname, port),
		)

		// Start the server in the background. You will need to wait a
		// small amount of time for it to bind to the localhost port
		// before you can start sending connections.
		go tunnel.Start()
		time.Sleep(100 * time.Millisecond)

		return cfg.New(ctx, "tcp", fmt.Sprintf("%s:%d", "127.0.0.1", tunnel.Local.Port))
	} else {
		return cfg.New(ctx, "tcp", fmt.Sprintf("%s:%s", hostname, port))
	}
}

func resourceKeysetCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client, err := getRedisConnection(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	keyset := d.Get("keyset").(map[string]interface{})

	for key, value := range keyset {
		err := client.Do(ctx, radix.FlatCmd(nil, "SET", key, value))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	hash, err := hashstructure.Hash(keyset, hashstructure.FormatV2, nil)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(fmt.Sprintf("%d", hash))

	return resourceKeysetRead(ctx, d, m)
}

func resourceKeysetRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	client, err := getRedisConnection(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	keyset := d.Get("keyset").(map[string]interface{})
	returnValues := make(map[string]interface{})

	for key := range keyset {
		var value string

		err := client.Do(ctx, radix.Cmd(&value, "GET", key))
		if err != nil {
			return diag.FromErr(err)
		}

		returnValues[key] = value
	}

	d.Set("keyset", returnValues)

	return diags
}

func resourceKeysetUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client, err := getRedisConnection(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	id := d.Id()
	if d.HasChanges("keyset") {
		err := client.Do(ctx, radix.WithConn(id, func(ctx context.Context, c radix.Conn) error {
			log.Printf("[TRACE] Calling MULTI")
			if err := c.Do(ctx, radix.Cmd(nil, "MULTI")); err != nil {
				return err
			}

			var err error
			defer func() {
				if err != nil {
					log.Printf("[TRACE] Calling DISCARD")
					c.Do(ctx, radix.Cmd(nil, "DISCARD"))
				}
			}()

			changes, _ := diff.Diff(d.GetChange("keyset"))
			log.Printf("[DEBUG] Diff %v", changes)
			for _, change := range changes {
				switch change.Type {
				case "create":
					log.Printf("[TRACE] Calling create")
					err = c.Do(ctx, radix.Cmd(nil, "SET", change.Path[0], change.To.(string)))
				case "update":
					log.Printf("[TRACE] Calling update")
					err = c.Do(ctx, radix.Cmd(nil, "SET", change.Path[0], change.To.(string)))
				case "delete":
					log.Printf("[TRACE] Calling delete")
					err = c.Do(ctx, radix.Cmd(nil, "DEL", change.Path[0]))
				}
			}

			log.Printf("[TRACE] Calling EXEC (unless defer works)")
			return c.Do(ctx, radix.Cmd(nil, "EXEC"))
		}))

		if err != nil {
			log.Printf("Err: %v", err)
			return diag.FromErr(err)
		}
	}

	return resourceKeysetRead(ctx, d, m)
}

func resourceKeysetDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	client, err := getRedisConnection(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	oldChange, _ := d.GetChange("keyset")
	keyset := oldChange.(map[string]interface{})

	for key := range keyset {
		err := client.Do(ctx, radix.FlatCmd(nil, "DEL", key))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId("")

	return diags
}
