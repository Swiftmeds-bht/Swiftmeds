# Substitution decision table

The service suggests candidates only when they share the source product's molecule.
Controlled products, the source product itself, and products with a different
molecule are excluded.

“Prescription-only” means either the source or candidate has
`requires_prescription = true`. The result classifies a suggestion; it does not
authorize dispensing or replace a lawful prescription.

| Same molecule? | Same strength? | Same dosage form? | Prescription-only on either product? | Result |
|---|---|---|---|---|
| No | Any | Any | Any | Exclude |
| Yes | Yes | Yes | No | `EXACT` |
| Yes | No | Yes | No | `NEEDS_PHARMACIST_CHECK` |
| Yes | Yes | No | No | `NEEDS_PHARMACIST_CHECK` |
| Yes | No | No | No | `NEEDS_PHARMACIST_CHECK` |
| Yes | Yes | Yes | Yes | `NEEDS_PHARMACIST_CHECK` |
| Yes | No | Any | Yes | `NEEDS_PHARMACIST_CHECK` |
| Yes | Any | No | Yes | `NEEDS_PHARMACIST_CHECK` |

Controlled source or candidate products and the source product itself are excluded
regardless of the table above. Exact matches are returned before candidates needing
pharmacist review.