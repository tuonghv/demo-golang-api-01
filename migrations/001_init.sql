CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    username      text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id          bigserial PRIMARY KEY,
    name        text NOT NULL,
    price_cents integer NOT NULL CHECK (price_cents >= 0),
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO products (name, price_cents) VALUES
    ('Mechanical Keyboard', 8999),
    ('USB-C Cable 1m', 1299),
    ('27-inch Monitor', 24999),
    ('Wireless Mouse', 3499),
    ('Laptop Stand', 4599),
    ('Webcam 1080p', 5999);
