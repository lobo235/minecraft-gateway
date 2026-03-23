job "minecraft-gateway" {
  node_pool   = "default"
  datacenters = ["dc1"]
  type        = "service"

  group "minecraft-gateway" {
    count = 1

    network {
      port "http" {
        to = 8080
      }
    }

    service {
      name     = "minecraft-gateway"
      port     = "http"
      provider = "consul"
      tags = [
        "traefik.enable=true",
        "traefik.http.routers.minecraft-gateway.rule=Host(`minecraft-gateway.example.com`)",
        "traefik.http.routers.minecraft-gateway.entrypoints=websecure",
        "traefik.http.routers.minecraft-gateway.tls=true",
      ]

      check {
        type     = "http"
        path     = "/health"
        port     = "http"
        interval = "30s"
        timeout  = "5s"

        check_restart {
          limit = 3
          grace = "30s"
        }
      }
    }

    restart {
      attempts = 3
      interval = "2m"
      delay    = "15s"
      mode     = "fail"
    }

    vault {
      cluster     = "default"
      change_mode = "restart"
    }

    task "minecraft-gateway" {
      driver = "docker"

      config {
        image = "ghcr.io/lobo235/minecraft-gateway:latest"
        ports = ["http"]
        volumes = [
          "/path/to/minecraft:/mnt/minecraft",
          "/path/to/data:/data",
        ]
      }

      template {
        data = <<EOF
{{ with secret "kv/data/nomad/default/minecraft-gateway" }}
GATEWAY_API_KEY={{ .Data.data.gateway_api_key }}
NOMAD_GATEWAY_KEY={{ .Data.data.nomad_gateway_key }}
VAULT_GATEWAY_KEY={{ .Data.data.vault_gateway_key }}
{{ end }}
EOF
        destination = "secrets/minecraft-gateway.env"
        env         = true
      }

      env {
        PORT              = "8080"
        LOG_LEVEL         = "info"
        NFS_BASE_PATH     = "/mnt/minecraft"
        DATA_DIR          = "/data"
        NOMAD_GATEWAY_URL = "http://nomad-gateway.example.com"
        VAULT_GATEWAY_URL = "http://vault-gateway.example.com"
      }

      resources {
        cpu    = 200
        memory = 128
      }

      kill_timeout = "35s"
    }
  }
}
