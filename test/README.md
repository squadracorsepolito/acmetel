# Testing the Pipeline

## Prerequirements

-   You need to have [Go](https://golang.org) installed.
-   You need to have [Docker](https://www.docker.com) installed and running.

## Running the Tests

From the current directory (`./test`) do the following steps:

-   Run the containers defined in the `docker-compose.yml` file:

    ```bash
    docker compose up -d
    ```

    This command will start:

    -   `QuestDB` to store the signals
    -   `Grafana` to visualize the signals and metrics
    -   `Prometheus` to collect metrics
    -   `Tempo` to collect traces
    -   `OpenTelemetry Collector` to collect open telemetry metrics/traces and forward them to `Prometheus`/`Tempo`

> [!WARNING] > `QuestDB` may give you a warning in the console about the `Max virtual memory areas limit`. If so, you should update your system settings as described in the fallowing [link](https://questdb.com/docs/operations/capacity-planning/#max-virtual-memory-areas-limit).

-   Run the server from the `server` folder:

    ```bash
    cd server
    go run .
    ```

    This command will start the server.

-   Run the client from the `client` folder:

    ```bash
    cd client
    go run .
    ```

    This command will start the client. The cliend sends **100,000** UDP packets to the server with rate of 1000 packets per second. Each packet contains **50** CAN messages. Each CAN message contains **8** signals of **8** bits. The value of the signals is a random number between `0` and `255` and it is the same for all the signals within a CAN message.

## Visualizing the Results

### Signals

The signals can be visualized in `Grafana` at `localhost:3000` in the `Signals` dashboard. The default credentials for `Grafana` are `admin/admin`.

If you prefer you can use directly the `QuestDB` console at `localhost:9000` to visualize the signals stored in the database.

### Metrics

The metrics can be visualized in `Grafana` in the `Acmetel Server` dashboard. Here you can find metrics like the processing time for each packet (message), the number of workers...

### Traces

The traces can be visualized in `Grafana` at the bottom of the `Acmetel Server` dashboard. Here are shown only the traces for packets (messages) that took more than 100ms to process. If you want to trace the entire journey of a packet (message) into the pipeline, click on the link in the first column.
