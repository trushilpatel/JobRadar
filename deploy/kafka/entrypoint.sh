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

# Start Kafka in background, wait for it to be ready, then create topics
/etc/confluent/docker/run &
KAFKA_PID=$!

echo "Waiting for Kafka to be ready..."
until kafka-topics --bootstrap-server localhost:9092 \
    --command-config /tmp/client.properties \
    --list > /dev/null 2>&1; do
  sleep 2
done

cat > /tmp/client.properties << PROPS
sasl.mechanism=PLAIN
security.protocol=SASL_PLAINTEXT
sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required username="${KAFKA_USER}" password="${KAFKA_PASSWORD}";
PROPS

for TOPIC in job_listings.raw job_listings.filtered; do
  kafka-topics --bootstrap-server localhost:9092 \
    --command-config /tmp/client.properties \
    --create --if-not-exists \
    --topic "$TOPIC" \
    --partitions 1 \
    --replication-factor 1
  echo "Topic ready: $TOPIC"
done

# Hand control back to Kafka process
wait $KAFKA_PID
