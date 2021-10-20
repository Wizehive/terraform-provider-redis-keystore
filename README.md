# Redis Keystore Terraform Provider

This provider allows controlling keys and values in a redis database. It's used for
situations where configuration values for services are stored in redis. Keys/values
can be based on other values in the terraform workspace and be applied directly if/when
something changes.

## Example Usage

```hcl
terraform {
  required_providers {
    redis-keystore = {
      source = "wizehive/redis-keystore"
    }
  }
}

provider "redis-keystore" {}

resource "redis-keystore_keyset" "test_keyset" {
  keyset = {
    "/test"                   = "testing",
    "/more/testing"           = "testing",
    "/more/more/more/testing" = "other stuff",
  }

  hostname = "localhost"
  port     = 6379
}
```

## Manual Install

Run the following command to install the vendor dependencies

```shell
go mod vendor
```

Run the following command to build the provider

```shell
make build
```

## Building / Releasing

- Tag release w/ v1.2.3 in git
- Run: `GPG_FINGERPRINT="02C557B75994954C9439E6E97065EE61B739A364" goreleaser release --rm-dist`
  - It will use the latest tag by default
