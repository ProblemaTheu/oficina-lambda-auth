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

# O balanceador é criado pelo Kubernetes, não pelo Terraform, então o DNS não
# sai de nenhum state. Vem de:
#   kubectl -n oficina-prod get svc api -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
# Se o Service for recriado, o nome muda e esta variável precisa acompanhar.
variable "backend_url" {
  description = "URL do balanceador que expõe a aplicação no EKS"
  type        = string
  default     = "http://adf501e528a5c4a999da812ef487526f-556148891.us-east-1.elb.amazonaws.com"
}
