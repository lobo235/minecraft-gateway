job "minecraft-gateway" {
  datacenters = ["dc1"]
  type        = "service"
  node_pool   = "default"

  group "minecraft-gateway" {
    count = 1

    network {
      port "http" {
        static = 8080
      }
    }

    task "minecraft-gateway" {
      driver = "docker"

      config {
        image = "ghcr.io/lobo235/minecraft-gateway:latest"
        ports = ["http"]
        volumes = [
          "/mnt/data/minecraft:/mnt/data/minecraft",
          "/mnt/data/homelab-ai/minecraft-gateway/data:/data",
        ]
      }

      env {
        PORT          = "${NOMAD_PORT_http}"
        NFS_BASE_PATH = "/mnt/data/minecraft"
        DATA_DIR      = "/data"
      }

      template {
        data        = <<-EOF
        {{ with nomadVar "nomad/jobs/minecraft-gateway" }}
        GATEWAY_API_KEY={{ .gateway_api_key }}
        NOMAD_GATEWAY_URL={{ .nomad_gateway_url }}
        NOMAD_GATEWAY_KEY={{ .nomad_gateway_key }}
        VAULT_GATEWAY_URL={{ .vault_gateway_url }}
        VAULT_GATEWAY_KEY={{ .vault_gateway_key }}
        {{ end }}
        EOF
        destination = "secrets/env.env"
        env         = true
      }

      resources {
        cpu    = 200
        memory = 128
      }

      service {
        name = "minecraft-gateway"
        port = "http"

        tags = [
          "traefik.enable=true",
          "traefik.http.routers.minecraft-gateway.rule=Host(`minecraft-gateway.example.com`)",
        ]

        check {
          type     = "http"
          path     = "/health"
          interval = "15s"
          timeout  = "5s"
        }
      }
    }
  }
}
