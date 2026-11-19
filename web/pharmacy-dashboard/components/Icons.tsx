// Small inline icons, the same shapes as the patient app's (Lucide). Decorative
// only: always aria-hidden, so the text next to each icon carries the meaning.
type IconProps = { size?: number };

function Svg({ size = 18, children }: IconProps & { children: React.ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export const BellIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" />
    <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
  </Svg>
);

export const ChevronDownIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="m6 9 6 6 6-6" />
  </Svg>
);

export const UploadIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M12 15V3M7 8l5-5 5 5" />
    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
  </Svg>
);

/** The SwiftMed mark, as in the patient app. */
export const BrandMark = ({ size = 28 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 28 28" aria-hidden="true" focusable="false">
    <rect width="28" height="28" rx="8" fill="currentColor" />
    <path d="M14 8v12M8 14h12" stroke="#fff" strokeWidth="3.2" strokeLinecap="round" />
  </svg>
);
