# Demo seed data

These CSV files are development fixtures for SwiftMeds:

- `medicines.csv`: 50 product records.
- `pharmacies.csv`: 15 pharmacy records, five each in Chukha, Paro, and Thimphu.
- `stock.csv`: 30 pharmacy/product stock records.

`stock.csv` references IDs from the other two files. Quantities are nonnegative integers; `0` represents no stock and `1` represents one unit remaining.

These are demo records. Pharmacy contact and license fields, demo brand labels, and stock quantities do not represent real registrations, contact details, or current inventory. Use only for development and testing.