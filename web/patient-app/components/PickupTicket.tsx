import { Fragment, type CSSProperties } from "react";

interface Props {
  code: string;
  /** Compact version for lists (My holds) and the example on the home page. */
  small?: boolean;
  /** The hold has just turned active: the digits rise into place once, in order. */
  settle?: boolean;
  /** The time left, under the tear line. */
  children?: React.ReactNode;
}

// The pickup code as a ticket: the one thing the patient shows at the counter.
// The code is drawn with spaces for the eye and read digit by digit for screen readers.
export function PickupTicket({ code, small = false, settle = false, children }: Props) {
  const digits = code.split("");
  const classes = ["ticket", small && "ticket-small", settle && "ticket-settle"].filter(Boolean).join(" ");
  return (
    <div className={classes}>
      <div className="ticket-top">
        <p className="pickup-label">Pickup code</p>
        <p className="pickup-code" aria-hidden="true">
          {digits.map((digit, i) => (
            <Fragment key={i}>
              {i > 0 && " "}
              <span style={{ "--i": i } as CSSProperties}>{digit}</span>
            </Fragment>
          ))}
        </p>
        {/* Read digit by digit, not as "four hundred eighty-two thousand…". */}
        <p className="sr-only">{digits.join(", ")}</p>
      </div>
      {children && (
        <>
          <div className="ticket-tear" aria-hidden="true" />
          <div className="ticket-bottom">{children}</div>
        </>
      )}
    </div>
  );
}
