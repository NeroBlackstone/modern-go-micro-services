storage "file" {
  path = "/vault/file"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = 1  # 开发环境禁用 TLS
}

api_addr = "http://0.0.0.0:8200"
