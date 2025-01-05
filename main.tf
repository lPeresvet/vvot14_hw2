terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
  required_version = ">= 0.13"
}

locals {
  home = "/home/www"
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
  execution_timeout  = 10
  environment = {
    "QUEUE_NAME" = local.queue_name,
    "AWS_ACCESS_KEY_ID"=yandex_iam_service_account_static_access_key.queue-static-key.access_key
    "AWS_SECRET_ACCESS_KEY"=yandex_iam_service_account_static_access_key.queue-static-key.secret_key
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