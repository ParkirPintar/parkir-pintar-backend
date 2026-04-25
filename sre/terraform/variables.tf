variable "db_username" {
  type      = string
  sensitive = true
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "mq_username" {
  type      = string
  sensitive = true
}

variable "mq_password" {
  type      = string
  sensitive = true
}

variable "istio_elb_name" {
  type        = string
  description = "Istio Ingress Gateway Classic ELB name (set after Istio install, leave empty during destroy)"
  default     = ""
}

  type        = string
  description = "GitHub organization or username, e.g. my-org"
}

variable "github_repo" {
  type        = string
  description = "GitHub repository name, e.g. parkir-pintar"
}
