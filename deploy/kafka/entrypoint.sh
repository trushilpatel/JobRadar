#!/bin/bash
set -e

# Generate JAAS config from environment variables
cat > /etc/kafka/kafka_server_jaas.conf << EOF
KafkaServer {
    org.apache.kafka.common.security.plain.PlainLoginModule required
    username="${KAFKA_USER}"
    password="${KAFKA_PASSWORD}"
    user_${KAFKA_USER}="${KAFKA_PASSWORD}";
};
EOF

# Start Kafka
exec /etc/confluent/docker/run
