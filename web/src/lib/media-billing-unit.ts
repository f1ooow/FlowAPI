export function billingUnitLabel(unit: string | undefined, t: (key: string) => string): string {
  switch (unit) {
    case 'seconds': case 'output_seconds': case 'h3_total_seconds': case 'h3_media_v2': return t('seconds')
    case 'minutes': case 'per-minute': return t('minutes')
    case 'input_minutes': case 'per-input-minute': return t('input minutes')
    case 'output_minutes': case 'per-output-minute': return t('output minutes')
    case 'characters': return t('characters')
    case 'images': return t('images')
    case 'tier': return t('resolution/duration tier')
    case 'tokens': return t('tokens')
    case 'free': return t('Free')
    case 'expression': return t('Dynamic Pricing')
    default: return t('request')
  }
}
