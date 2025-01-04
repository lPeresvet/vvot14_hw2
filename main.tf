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

resource "yandex_storage_object" "model_setup1" {
  bucket = yandex_storage_bucket.input-bucket.id
  key    = "model/dlib_face_recognition_resnet_model_v1.dat"
  source = "./model/dlib_face_recognition_resnet_model_v1.dat"
}

resource "yandex_storage_object" "model_setup1" {
  bucket = yandex_storage_bucket.input-bucket.id
  key    = "model/shape_predictor_5_face_landmarks.dat"
  source = "./model/shape_predictor_5_face_landmarks.dat"
}

resource "yandex_function" "face-detect" {
  name        = "vvot14-face-detection"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.Handler"
  memory      = 128
  execution_timeout  = 10

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