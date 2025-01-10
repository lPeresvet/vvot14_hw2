terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
  required_version = ">= 0.13"
}

locals {
  home = "/Users/kirill"
  queue_name = "vvot14-task"
}

variable "cloud_id" {
  type = string
  description = "Идентификатор облака"
}

variable "folder_id" {
  type = string
  description = "Идентификатор каталога"
}

variable "TG_API_KEY" {
  type = string
  description = "Ключ телеграмм бота"
}

provider "yandex" {
  cloud_id = var.cloud_id
  folder_id = var.folder_id
  service_account_key_file = "${local.home}/.yc-keys/key.json"
}

resource "yandex_iam_service_account" "func-bot-account" {
  name        = "func-bot-account"
  description = "Аккаунт для функции"
  folder_id   = var.folder_id
}

resource "yandex_iam_service_account_static_access_key" "queue-static-key" {
  service_account_id = yandex_iam_service_account.func-bot-account.id
  description        = "Ключ для очереди"
}

resource "yandex_resourcemanager_folder_iam_binding" "mount-iam" {
  folder_id = var.folder_id
  role               = "editor"

  members = [
    "serviceAccount:${yandex_iam_service_account.func-bot-account.id}",
  ]
}

resource "archive_file" "zip" {
  type = "zip"
  output_path = "src.zip"
  source_dir = "internal/face_detection"
}

resource "yandex_storage_bucket" "input-bucket" {
  bucket = "vvot14-photo"
  folder_id = var.folder_id
}

resource "yandex_function" "face-detect" {
  name        = "vvot14-face-detection"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.Handler"
  memory      = 128
  execution_timeout  = 15
  environment = {
    "QUEUE_URL" = yandex_message_queue.task_queue.id,
    "AWS_ACCESS_KEY_ID"=yandex_iam_service_account_static_access_key.queue-static-key.access_key
    "AWS_SECRET_ACCESS_KEY"=yandex_iam_service_account_static_access_key.queue-static-key.secret_key,
    "API_GW_URL" = yandex_api_gateway.api-gateway.domain
  }

  service_account_id = yandex_iam_service_account.func-bot-account.id

  storage_mounts {
    mount_point_name = "images"
    bucket = yandex_storage_bucket.input-bucket.bucket
    prefix           = ""
  }

  content {
    zip_filename = archive_file.zip.output_path
  }
}

resource "yandex_function_trigger" "input_trigger" {
  name        = "vvot14-photo"
  description = "Триггер для запуска обработчика vvot14-face-detection"
  function {
    id                 = yandex_function.face-detect.id
    service_account_id = yandex_iam_service_account.func-bot-account.id
    retry_attempts     = 2
    retry_interval = 10
  }
  object_storage {
    bucket_id    = yandex_storage_bucket.input-bucket.id
    suffix       = ".jpg"
    create       = true
    update       = false
    delete       = false
    batch_cutoff = 2
  }
}

resource "yandex_message_queue" "task_queue" {
  name                        = local.queue_name
  visibility_timeout_seconds  = 600
  receive_wait_time_seconds   = 20
  message_retention_seconds   = 1209600
  access_key = yandex_iam_service_account_static_access_key.queue-static-key.access_key
  secret_key = yandex_iam_service_account_static_access_key.queue-static-key.secret_key
}

resource "yandex_storage_bucket" "faces-bucket" {
  bucket = "vvot14-faces"
  folder_id = var.folder_id
}

resource "archive_file" "faces-src" {
  type = "zip"
  output_path = "faces-src.zip"
  source_dir = "internal/face_cut"
}

resource "yandex_function" "face-cut" {
  name        = "vvot14-face-cut"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.Handler"
  memory      = 128
  execution_timeout  = 10
  environment = {
    "YDB_URL" = yandex_ydb_database_serverless.face-img-db.ydb_full_endpoint,
    "AWS_ACCESS_KEY_ID"=yandex_iam_service_account_static_access_key.queue-static-key.access_key
    "AWS_SECRET_ACCESS_KEY"=yandex_iam_service_account_static_access_key.queue-static-key.secret_key
  }

  service_account_id = yandex_iam_service_account.func-bot-account.id

  storage_mounts {
    mount_point_name = "images"
    bucket = yandex_storage_bucket.input-bucket.bucket
    prefix           = ""
  }

  storage_mounts {
    mount_point_name = "faces"
    bucket = yandex_storage_bucket.faces-bucket.bucket
    prefix           = ""
  }

  content {
    zip_filename = archive_file.faces-src.output_path
  }
}

resource "yandex_function_trigger" "ymq_trigger" {
  name        = "vvot14-task"

  message_queue {
    queue_id = yandex_message_queue.task_queue.arn
    batch_cutoff = "5"
    batch_size = "5"
    service_account_id = yandex_iam_service_account.func-bot-account.id
  }
  function {
    id = yandex_function.face-cut.id
    service_account_id = yandex_iam_service_account.func-bot-account.id
  }
}

resource "yandex_api_gateway" "api-gateway" {
  name        = "vvot14-apigw"
  description = "API - шлюз для доступа к бакету faces"
  labels      = {
    label       = "label"
    empty-label = ""
  }
  spec = <<-EOT
    openapi: "3.0.0"
    info:
      version: 1.0.0
      title: Face API
    paths:
      /:
        get:
          summary: Serve static file from Yandex Cloud Object Storage
          parameters:
            - name: face
              in: query
              required: false
              schema:
                type: string
            - name: image
              in: query
              required: false
              schema:
                type: string
          responses:
            "200":
              description: File
              content:
                image/jpeg:
                  schema:
                    type: string
                    format: binary
          x-yc-apigateway-integration:
            type: cloud_functions
            payload_format_version: '0.1'
            function_id: ${yandex_function.api-gw.id}
            tag: $latest
            service_account_id: ${yandex_iam_service_account.func-bot-account.id}
  EOT
}

resource "yandex_ydb_database_serverless" "face-img-db" {
  name                = "vvot14-db-photo-face"
  deletion_protection = false

  serverless_database {
    enable_throttling_rcu_limit = false
    provisioned_rcu_limit       = 10
    storage_size_limit          = 50
    throttling_rcu_limit        = 0
  }
}

resource "yandex_ydb_table" "relations_table" {
  path = "relations"
  connection_string = yandex_ydb_database_serverless.face-img-db.ydb_full_endpoint

  column {
    name = "ImageID"
    type = "String"
    not_null = true
  }
  column {
    name = "FaceID"
    type = "String"
    not_null = true
  }

  primary_key = ["FaceID"]
}

resource "yandex_ydb_table" "names_table" {
  path = "names"
  connection_string = yandex_ydb_database_serverless.face-img-db.ydb_full_endpoint

  column {
    name = "FaceName"
    type = "String"
    not_null = false
  }
  column {
    name = "FaceID"
    type = "String"
    not_null = true
  }

  primary_key = ["FaceID"]
}

resource "archive_file" "bot" {
  type = "zip"
  output_path = "bot_src.zip"
  source_dir = "internal/tg_bot"
}

resource "yandex_function_iam_binding" "function-iam" {
  function_id = yandex_function.bot.id
  role        = "serverless.functions.invoker"

  members = [
    "system:allUsers",
  ]
}

resource "yandex_function" "bot" {
  name        = "vvot14-boot"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.Handler"
  memory      = 128
  execution_timeout  = 10
  environment = {
    "TG_API_KEY" = var.TG_API_KEY,
    "YDB_URL" = yandex_ydb_database_serverless.face-img-db.ydb_full_endpoint,
    "API_GW_URL" = yandex_api_gateway.api-gateway.domain
  }
  service_account_id = yandex_iam_service_account.func-bot-account.id

  storage_mounts {
    mount_point_name = "faces"
    bucket = yandex_storage_bucket.faces-bucket.bucket
    prefix           = ""
  }

  storage_mounts {
    mount_point_name = "images"
    bucket = yandex_storage_bucket.input-bucket.bucket
    prefix           = ""
  }

  content {
    zip_filename = archive_file.bot.output_path
  }
}

resource "yandex_function" "api-gw" {
  name        = "vvot14-api-gw"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.Handler"
  memory      = 128
  execution_timeout  = 10
  service_account_id = yandex_iam_service_account.func-bot-account.id

  storage_mounts {
    mount_point_name = "faces"
    bucket = yandex_storage_bucket.faces-bucket.bucket
    prefix           = ""
  }

  storage_mounts {
    mount_point_name = "images"
    bucket = yandex_storage_bucket.input-bucket.bucket
    prefix           = ""
  }

  content {
    zip_filename = archive_file.gw.output_path
  }
}

resource "archive_file" "gw" {
  type = "zip"
  output_path = "gw_src.zip"
  source_dir = "internal/api_gw"
}

resource "null_resource" "curl" {
  provisioner "local-exec" {
    command = "curl --insecure -X POST https://api.telegram.org/bot${var.TG_API_KEY}/setWebhook?url=https://functions.yandexcloud.net/${yandex_function.bot.id}"
  }

  triggers = {
    on_version_change = var.TG_API_KEY
  }

  provisioner "local-exec" {
    when    = destroy
    command = "curl --insecure -X POST https://api.telegram.org/bot${self.triggers.on_version_change}/deleteWebhook"
  }
}