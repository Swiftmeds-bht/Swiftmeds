# Demo seed data

These CSV files are development fixtures for SwiftMeds:

- `medicines.csv`: 50 product records.
- `pharmacies.csv`: 15 pharmacy records, five each in Chukha, Paro, and Thimphu.
- `stock.csv`: 30 pharmacy/product stock records.
- `prices.demo.csv`: deterministic synthetic prices for the 30 seeded pharmacy/product pairs.

`stock.csv` references IDs from the other two files. Quantities are nonnegative integers; `0` represents no stock and `1` represents one unit remaining.

These are demo records. Pharmacy contact and license fields, demo brand labels, and stock quantities do not represent real registrations, contact details, or current inventory. Use only for development and testing.

Run `make seed` from the repository root after copying `.env.example` to `.env`.
The target starts the Catalog, Pharmacy Registry, Geo, Inventory, and Pricing
services, waits for their migrations, and imports the fixtures transactionally.
Catalog medicines are already loaded by
`services/catalog/migrations/002_seed_medicines.up.sql`; `make seed` does not
import them a second time.

The Pharmacy Registry schema stores the pharmacy ID, owner, name, license,
coordinates, status, and timestamps. It does not have columns for the CSV
phone, address, or dzongkhag fields, so those fields remain fixture metadata
and are not claimed to be persisted. Coordinates are also loaded into Geo.
Repeated seeding updates only the demo rows. Inventory preserves any existing
`quantity_reserved` value and never truncates stock or reservation tables.

Prices are synthetic `amount_chhertum` values solely for development and testing;
they are not real pharmacy prices.