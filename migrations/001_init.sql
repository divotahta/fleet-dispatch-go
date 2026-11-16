CREATE DATABASE IF NOT EXISTS fleet_dispatch
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

USE fleet_dispatch;

CREATE TABLE IF NOT EXISTS drivers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    nama VARCHAR(100) NOT NULL,
    nomor_lambung VARCHAR(30) NOT NULL,
    status ENUM('available', 'on_trip', 'offline') NOT NULL DEFAULT 'available',
    lat DOUBLE NOT NULL,
    lng DOUBLE NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT uq_drivers_nomor_lambung UNIQUE (nomor_lambung),
    INDEX idx_drivers_status (status)
);

CREATE TABLE IF NOT EXISTS orders (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    driver_id BIGINT NOT NULL,
    penumpang VARCHAR(100) NOT NULL,
    pickup_lat DOUBLE NOT NULL,
    pickup_lng DOUBLE NOT NULL,
    dropoff_lat DOUBLE NOT NULL,
    dropoff_lng DOUBLE NOT NULL,
    jarak_km DOUBLE NOT NULL,
    tarif INT NOT NULL,
    status ENUM('assigned', 'picked_up', 'completed', 'cancelled') NOT NULL DEFAULT 'assigned',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_orders_driver FOREIGN KEY (driver_id) REFERENCES drivers (id),
    INDEX idx_orders_driver_status (driver_id, status)
);
