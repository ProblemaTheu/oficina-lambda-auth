# Descoberta dos balanceadores dos três microsserviços.
#
# Na Fase 3 o DNS do CLB do OS era um default fixo em variables.tf, que
# quebrava a cada recriação do Service ou da conta. O controlador de serviço
# do EKS marca cada Classic Load Balancer com a tag
#   kubernetes.io/service-name = <namespace>/<service>
# e é por ela que achamos o balanceador — o Terraform segue o cluster sem
# ninguém copiar DNS.
#
# Consequência de ordem: aplique este repositório DEPOIS do primeiro deploy
# dos três serviços (o CLB nasce no `kubectl apply` do CD). Se algum ainda
# não existir, o plan falha com a mensagem do `precondition` abaixo.

locals {
  services_k8s = {
    os        = "oficina-prod/api"
    billing   = "billing-prod/billing"
    execution = "execution-prod/execution"
  }
}

data "aws_resourcegroupstaggingapi_resources" "lb" {
  for_each = { for k, v in local.services_k8s : k => v if !contains(keys(var.backend_urls), k) }

  resource_type_filters = ["elasticloadbalancing:loadbalancer"]
  tag_filter {
    key    = "kubernetes.io/service-name"
    values = [each.value]
  }
}

locals {
  # ARN de CLB: arn:aws:elasticloadbalancing:<região>:<conta>:loadbalancer/<nome>
  # (ELBv2 teria loadbalancer/app|net/<nome>/<id> — não usamos: ver o overlay
  # prod de cada serviço).
  lb_nome = {
    for k, d in data.aws_resourcegroupstaggingapi_resources.lb :
    k => try(element(split("/", d.resource_tag_mapping_list[0].resource_arn), 1), "")
  }
}

data "aws_elb" "backend" {
  for_each = local.lb_nome
  name     = each.value

  lifecycle {
    precondition {
      condition     = each.value != ""
      error_message = "Nenhum Classic Load Balancer com a tag kubernetes.io/service-name=${local.services_k8s[each.key]}. Faça o primeiro deploy do serviço (CD) antes de aplicar este repositório, ou informe backend_urls.${each.key}."
    }
  }
}

locals {
  backend = merge(
    { for k, lb in data.aws_elb.backend : k => "http://${lb.dns_name}" },
    var.backend_urls,
  )
}
