import type { SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement> & { size?: number }

function icon(paths: string[]) {
  return function Icon({ size = 18, ...props }: IconProps) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
        {...props}
      >
        {paths.map((path) => (
          <path key={path} d={path} />
        ))}
      </svg>
    )
  }
}

export const SearchIcon = icon(['M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14Z', 'm20 20-3.5-3.5'])
export const UploadIcon = icon([
  'M12 16V4',
  'M7 9l5-5 5 5',
  'M4 16v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3',
])
export const DownloadIcon = icon(['M12 3v12', 'M7 10l5 5 5-5', 'M5 21h14'])
export const TrashIcon = icon(['M4 7h16', 'M9 7V4h6v3', 'M6 7l1 13h10l1-13'])
export const RestoreIcon = icon(['M3 12a9 9 0 1 0 3-6.7L3 8', 'M3 3v5h5'])
export const RefreshIcon = icon(['M21 12a9 9 0 1 1-3-6.7L21 8', 'M21 3v5h-5'])
export const ChevronLeftIcon = icon(['m15 18-6-6 6-6'])
export const ChevronRightIcon = icon(['m9 18 6-6-6-6'])
export const ChevronUpIcon = icon(['m6 15 6-6 6 6'])
export const ChevronDownIcon = icon(['m6 9 6 6 6-6'])
export const CloseIcon = icon(['M6 6l12 12', 'M18 6 6 18'])
export const CheckIcon = icon(['m5 12 5 5L20 7'])
export const MenuIcon = icon(['M4 6h16', 'M4 12h16', 'M4 18h16'])
export const EditIcon = icon(['M4 20h4L19 9l-4-4L4 16z'])
export const WarningIcon = icon(['M12 3 2 21h20z', 'M12 10v5', 'M12 18v.5'])
export const ErrorIcon = icon(['M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z', 'M12 7v6', 'M12 16.5v.5'])
export const InfoIcon = icon(['M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z', 'M12 10v6', 'M12 7h.01'])
export const ClockIcon = icon(['M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z', 'M12 7v5l3 2'])
export const FolderIcon = icon([
  'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
])
export const PlusIcon = icon(['M12 5v14', 'M5 12h14'])
export const ImageIcon = icon([
  'M6 4h12a3 3 0 0 1 3 3v10a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3V7a3 3 0 0 1 3-3Z',
  'M9 12a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z',
  'm21 16-5-5-9 9',
])
export const GamepadIcon = icon([
  'M8 6h8a6 6 0 0 1 0 12H8A6 6 0 0 1 8 6Z',
  'M7 10v4',
  'M5 12h4',
  'M16 11h.01',
  'M18 13h.01',
])
export const MoreIcon = icon(['M5 12h.01', 'M12 12h.01', 'M19 12h.01'])
export const ArrowRightIcon = icon(['M5 12h14', 'M13 6l6 6-6 6'])
export const FileIcon = icon([
  'M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z',
  'M14 3v6h6',
])
export const PartsIcon = icon([
  'M4 5h16v14H4z',
  'M8 5v14',
  'M16 5v14',
  'M2 9h2',
  'M20 9h2',
  'M2 15h2',
  'M20 15h2',
])
export const SeriesIcon = icon(['M4 5h16v14H4z', 'M8 9h8', 'M8 13h8', 'M8 17h5'])
export const FilesIcon = icon([
  'M9 7h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2Z',
  'M15 7V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h2',
])
export const PauseIcon = icon(['M8 5v14', 'M16 5v14'])
export const PlayIcon = icon(['M8 5v14l11-7z'])
export const EyeIcon = icon([
  'M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6Z',
  'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z',
])
