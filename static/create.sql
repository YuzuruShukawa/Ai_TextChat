CREATE TABLE `personas` (
  `id` INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(64) NOT NULL,
  `avatar` VARCHAR(256) DEFAULT '/static/ai_avatar.png',
  `identity` VARCHAR(128),
  `appearance` TEXT,
  `personality` TEXT,
  `created_at` DATETIME,
  `updated_at` DATETIME
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `sessions` (
  `id` VARCHAR(64) NOT NULL PRIMARY KEY,
  `name` VARCHAR(64),
  `model` VARCHAR(64),
  `personality` TEXT,
  `ai_name` VARCHAR(64) DEFAULT 'AI助手',
  `ai_avatar` VARCHAR(256) DEFAULT '/static/ai_avatar.png',
  `terminated` TINYINT(1) DEFAULT 0,
  `created_at` DATETIME,
  `updated_at` DATETIME,
  `persona_id` INT UNSIGNED DEFAULT NULL,
  FOREIGN KEY (`persona_id`) REFERENCES `personas`(`id`) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `messages` (
  `id` INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `session_id` VARCHAR(64) NOT NULL,
  `role` VARCHAR(16),
  `content` TEXT,
  `meta` VARCHAR(128),
  `created_at` DATETIME,
  FOREIGN KEY (`session_id`) REFERENCES `sessions`(`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  INDEX (`session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;