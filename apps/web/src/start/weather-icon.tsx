// QWeather Icons v1.8.0 (MIT): local outline SVGs, including night variants.
// Source and license ship in public/weather/; never infer weather from UI theme.
const codes = new Set(['100', '101', '102', '103', '104', '150', '151', '152', '153', '300', '301', '302', '303', '304', '305', '306', '307', '308', '309', '310', '311', '312', '313', '314', '315', '316', '317', '318', '350', '351', '399', '400', '401', '402', '403', '404', '405', '406', '407', '408', '409', '410', '456', '457', '499', '500', '501', '502', '503', '504', '507', '508', '509', '510', '511', '512', '513', '514', '515', '900', '901', '999'])

export function WeatherIcon({ code, size = 20 }: { code?: string; size?: number }) {
  const resolved = code && codes.has(code) ? code : '999'
  return <svg width={size} height={size} viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" focusable="false"
    data-weather-icon={code ?? 'unavailable'} data-icon-code={resolved}>
    <use href={`/weather/qweather-icons.svg#weather-${resolved}`} />
  </svg>
}
