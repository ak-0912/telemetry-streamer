Telemetry Streamer: reads telemetry from CSV and streams it periodically over the custom message
queue. The implementation should support the ability to dynamically scale up/down the number of
Streamers.
    NOTE: The data to be streamed is provided as a CSV file. Each line in the CSV is an independent
    telemetry datapoint. This data can be streamed in a loop to simulate continuous stream of
    telemetry. The time at which a specific telemetry log is processed should be considered as the
    timestamp of that telemetry.


csv file name = dcgm_metrics_20250718_134233.csv

Telemetry streamer sends reads data from csv in a infinite loop and send to custom message queue. 
if message queue overloaded then send the message to stop for sometimes as queue is little overloaded to avoid back pressure.. 

use protocol butter for send message, create csv struct in domain

📌 Telemetry Streamer – Agentic Specification

Design and implement a Telemetry Streamer service with the following capabilities:

Core Behavior

The streamer reads telemetry data from a CSV file:

    dcgm_metrics_20250718_134233.csv
- Each row in the CSV represents an independent telemetry datapoint.
- The streamer continuously loops over the CSV data to simulate an infinite telemetry stream.
    - Read rows sequentially from the CSV.
    - When the end of file (EOF) is reached, restart from index 0 (beginning of file).
- The timestamp for each telemetry message must be assigned at processing time (current system time), not derived from the CSV.


Message Format

- Define a domain-level telemetry schema using Protocol Buffers.
- Convert each CSV row into a structured protobuf message before sending.
- Ensure schema is strongly typed and extensible.

Streaming Behavior
- Publish telemetry messages to a custom message queue.
- Messages should be sent at a configurable rate (e.g., interval-based streaming).

Backpressure Handling

    Continuously monitor queue health (e.g., queue depth, latency, or publish acknowledgments).
    If the queue is overloaded:
    Temporarily pause or throttle publishing.
    Resume automatically once the queue returns to a safe threshold.
    This mechanism must prevent backpressure and message loss.

    I handle backpressure using a dynamic throttling mechanism instead of a hard stop.
    The streamer continuously monitors queue health (like queue depth or consumer lag).

    Under normal conditions, it streams at a configured rate.
    As the queue starts filling up, it gradually reduces the send rate.
    If the queue is critically overloaded, it applies a temporary pause with exponential backoff.
    Once the queue recovers, it smoothly ramps up instead of flooding it again.

    This avoids oscillation and makes the system stable under load.

Scalability
    -The system must support horizontal scaling:
    - Multiple streamer instances can run in parallel.
    - Ability to dynamically scale up/down the number of streamer instances.
    - Ensure no tight coupling between instances (stateless design preferred).
    - Streamer replicas are dynamically scaled via Kubernetes HPA/KEDA; application workers remain fixed per pod with adaptive backpressure.

Additional Expectations
    -Clean separation between:
    - CSV ingestion
    -Transformation (CSV → protobuf)
    -Queue publishing


Configurable parameters:
    File path
    Streaming rate
    Backpressure thresholds
    Number of streamer instances