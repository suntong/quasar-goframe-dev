// Auto-generated validation utilities — do not edit manually.

type QRule = (val: any) => true | string;

export function required(label: string): QRule {
  return (val) => (val !== null && val !== undefined && val !== '') || label + ' is required';
}

export function minLength(label: string, min: number): QRule {
  return (val) => !val || String(val).length >= min || label + ' must be at least ' + min + ' characters';
}

export function maxLength(label: string, max: number): QRule {
  return (val) => !val || String(val).length <= max || label + ' must be at most ' + max + ' characters';
}

export function minValue(label: string, min: number): QRule {
  return (val) => val === '' || val === null || Number(val) >= min || label + ' must be >= ' + min;
}

export function maxValue(label: string, max: number): QRule {
  return (val) => val === '' || val === null || Number(val) <= max || label + ' must be <= ' + max;
}

export function pattern(label: string, regex: string): QRule {
  const re = new RegExp(regex);
  return (val) => !val || re.test(String(val)) || label + ' format is invalid';
}

export function email(label: string): QRule {
  return pattern(label, '^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$');
}
