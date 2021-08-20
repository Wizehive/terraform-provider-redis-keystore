terraform {
  required_providers {
    redis-keystore = {
      source = "local/wizehive/redis-keystore"
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
