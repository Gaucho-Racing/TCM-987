import can
import time
import threading
import os
import csv

# Optional CPU monitoring
try:
    import psutil
except Exception:
    psutil = None

CPU_MON_INTERVAL = float(os.environ.get("CPU_MON_INTERVAL", "0.1"))

CSV_FILE = "can_log.csv"


def _read_proc_cpu():
    try:
        with open('/proc/stat', 'r') as f:
            first = f.readline()

        parts = first.split()
        vals = [int(x) for x in parts[1:]]

        idle = vals[3]
        total = sum(vals)

        return total, idle

    except Exception:
        return None, None


def monitor_cpu(stop_event, interval=CPU_MON_INTERVAL):
    if psutil:
        while not stop_event.is_set():
            try:
                pct = psutil.cpu_percent(interval=interval)
                print(f"[SYS] CPU: {pct:.1f}%")

            except Exception as e:
                print(f"✗ CPU monitor error: {e}")
                time.sleep(interval)

        return

    prev_total, prev_idle = _read_proc_cpu()

    if prev_total is None:
        print("✗ CPU monitor unavailable (no psutil and /proc/stat missing)")
        return

    while not stop_event.is_set():
        time.sleep(interval)

        cur_total, cur_idle = _read_proc_cpu()

        if cur_total is None or prev_total is None:
            continue

        dt = cur_total - prev_total
        di = cur_idle - prev_idle

        prev_total, prev_idle = cur_total, cur_idle

        if dt <= 0:
            pct = 0.0
        else:
            pct = 100.0 * (1.0 - (di / dt))

        print(f"[SYS] CPU: {pct:.1f}%")


def receive_all():
    # Try to open can0, can1, and can2. If a device is missing, print an error but continue.
    buses = {}
    for idx in (0, 1, 2):
        name = f'can{idx}'
        try:
            bus = can.Bus(interface='socketcan', channel=name, bitrate=500000)
            print(f"Opened {name}")
            buses[idx] = bus
        except Exception as e:
            print(f"Could not open {name}: {e}")
            buses[idx] = None

    if not any(b for b in buses.values()):
        print("No CAN interfaces available (can0, can1, and can2). Exiting.")
        return

    print("Listening for messages... Press Ctrl+C to exit.")
    print(f"Logging CAN data to: {CSV_FILE}")

    # Start time for relative timestamps
    start_time = time.monotonic()

    # Open CSV file
    csv_file = open(CSV_FILE, mode='w', newline='', buffering=1)
    csv_writer = csv.writer(csv_file)

    # CSV header
    csv_writer.writerow([
        "Time Stamp",
        "ID",
        "Extended",
        "Dir",
        "Bus",
        "LEN",
        "D1",
        "D2",
        "D3",
        "D4",
        "D5",
        "D6",
        "D7",
        "D8"
    ])

    stop_event = threading.Event()

    cpu_thread = threading.Thread(target=monitor_cpu, args=(stop_event,), daemon=True)
    cpu_thread.start()
    # Create a lock for CSV writes
    csv_lock = threading.Lock()

    # Listener to be used with python-can Notifier
    class CSVListener(can.Listener):
        def __init__(self, idx, csv_writer, csv_file, csv_lock, start_time):
            super().__init__()
            self.idx = idx
            self.csv_writer = csv_writer
            self.csv_file = csv_file
            self.csv_lock = csv_lock
            self.start_time = start_time

        def on_message_received(self, msg):
            try:
                timestamp_ms = int((time.monotonic() - self.start_time) * 1000)
                can_id = f"{msg.arbitration_id:08X}"
                extended = "true" if msg.is_extended_id else "false"
                direction = "Rx"
                bus_number = self.idx
                length = msg.dlc
                data = [f"{byte:02X}" for byte in msg.data]
                data += ["00"] * (8 - len(data))

                with self.csv_lock:
                    self.csv_writer.writerow([
                        timestamp_ms,
                        can_id,
                        extended,
                        direction,
                        bus_number,
                        length,
                        *data[:8]
                    ])
                    self.csv_file.flush()

                print(
                    f"Bus: can{bus_number} | ID: {hex(msg.arbitration_id)} | "
                    f"Data: {list(msg.data)} | DLC: {msg.dlc}"
                )
            except Exception as e:
                print(f"Error in CSVListener for can{self.idx}: {e}")

    # Create notifiers (one per opened bus) using CSVListener
    notifiers = []
    for idx, bus in buses.items():
        if bus is None:
            continue
        listener = CSVListener(idx, csv_writer, csv_file, csv_lock, start_time)
        notifier = can.Notifier(bus, [listener], timeout=1.0)
        notifiers.append(notifier)

    try:
        # Main thread just waits for Ctrl+C
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nStopping listener...")
    finally:
        stop_event.set()
        cpu_thread.join(timeout=1)

        # Stop notifiers
        for notifier in notifiers:
            try:
                notifier.stop()
            except Exception:
                pass

        # Shutdown only opened buses
        for idx, bus in buses.items():
            if bus is not None:
                try:
                    bus.shutdown()
                except Exception:
                    pass

        csv_file.close()
        print(f"CSV saved to: {CSV_FILE}")


if __name__ == "__main__":
    receive_all()