import type { MedicineName } from "@/lib/names";

/** A medicine's name with its strength and form; the ID only when the name is unknown. */
export function MedicineCell({ medicine, productId }: { medicine: MedicineName; productId?: string }) {
  return (
    <div className="medicine-cell">
      <span className="medicine-name">{medicine.name}</span>
      {medicine.detail ? <span className="medicine-detail">{medicine.detail}</span> : null}
      {!medicine.known && productId ? <code className="id-value">{productId}</code> : null}
    </div>
  );
}
