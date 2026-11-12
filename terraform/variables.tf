variable "regiao" {
  description = "Região AWS"
  type        = string
  default     = "us-east-1"
}

variable "projeto" {
  description = "Prefixo dos recursos"
  type        = string
  default     = "oficina"
}

variable "ambiente" {
  description = "Ambiente único da Fase 3 (corte 10)"
  type        = string
  default     = "prod"
}

# Os balanceadores são criados pelo Kubernetes (Service type LoadBalancer),
# não pelo Terraform: o DNS é descoberto em backends.tf pela tag que o EKS
# põe em cada CLB. Esta variável só existe para forçar uma URL (teste, ou um
# balanceador fora do padrão) — chaves: os, billing, execution.
variable "backend_urls" {
  description = "Sobrescreve a URL descoberta de um backend (ex.: { os = \"http://...\" })"
  type        = map(string)
  default     = {}

  validation {
    condition     = alltrue([for k in keys(var.backend_urls) : contains(["os", "billing", "execution"], k)])
    error_message = "Chaves aceitas em backend_urls: os, billing, execution."
  }
}
