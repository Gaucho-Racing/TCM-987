# TCM-987
Telemetry Compute Module implementation for the 987 generation Porsche Cayman/Boxster

## Seeed Studio USB-CAN Analyzer

The Linux relay supports the Seeed Studio USB-CAN Analyzer's serial protocol.
It configures the adapter in silent (listen-only) mode and forwards received
frames through the existing SQLite and MQTT pipeline. The default CAN bitrate
is 500000 bps; the USB serial baud rate is 2000000. These are separate settings.
SocketCAN interfaces and virtual UDP inputs remain available.

The host must expose the adapter as a serial device, normally `/dev/ttyUSB0`.
For the CH340 USB ID `1a86:7523`, load the Linux `ch341` driver. If the Jetson's
kernel omits it, install or build the matching module for the running kernel;
the USB device appearing in `lsusb` alone is insufficient.

Copy `example.env` to `.env`, set the vehicle identity and broker credentials,
then start the development stack with the USB device mapping:

```sh
docker compose -f docker-compose.yml -f docker-compose.seeed.yml up --build
```

The example publishes to the bundled NanoMQ broker at `localhost:1883`.
Set `LOCAL_MQTT_HOST` and `LOCAL_MQTT_PORT` to use an external broker, and set
`LOCAL_MQTT_USER` and `LOCAL_MQTT_PASSWORD` if authentication is required.
Cloud publishing is disabled by default; configure `CLOUD_MQTT_HOST` and its
credentials to publish to a second broker.

| Setting | Meaning | Default |
| --- | --- | --- |
| `SEEED_CAN_DEVICE` | Host serial device mapped by the Compose override | `/dev/ttyUSB0` |
| `SEEED_CAN_PORTS` | Comma-separated `device:bus_label` pairs inside the relay | disabled |
| `SEEED_CAN_BITRATE` | CAN bus bitrate, shared by configured Seeed adapters | `500000` |
| `SEEED_SERIAL_BAUD` | Adapter's current serial baud rate | `2000000` |
| `SEEED_FRAME_TYPE` | Adapter frame selection, `STD` or `EXT` | `STD` |

The Compose override sets `SEEED_CAN_PORTS=/dev/ttyUSB0:pcan` and disables
SocketCAN inputs. For a native process, set that environment variable directly
and grant the process permission to access the serial device. Received topics
remain `p987/<vehicle_id>/pcan/0x<can_id>`. Device disconnects are retried every
five seconds; after unplugging and reconnecting a Docker-mapped device, recreate
the container if its device mapping is stale.

Protocol references: [Seeed's documentation](https://github.com/SeeedDocument/USB-CAN-Analyzer)
and the [python-can Seeed interface](https://python-can.readthedocs.io/en/stable/interfaces/seeedstudio.html).
