// Filename: lib/template-utils.ts

const PLACEHOLDER_REGEX = /{{\s*\.([a-zA-Z0-9_]+)\s*}}/g;

export type FormFieldType = 'text' | 'textarea' | 'carousel' | 'asset' | 'background';

export interface FormField {
  name: string;
  type: FormFieldType;
  label: string;
}

/**
 * Keywords that suggest a placeholder should be a multi-line textarea instead of a single-line input.
 */
const TEXTAREA_KEYWORDS = new Set([
  'text',
  'body',
  'content',
  'description',
  'title_constrained',
  'title_wrapping',
]);

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function findPlaceholdersInNode(node: unknown): string[] {
  const placeholders = new Set<string>();
  function traverse(n: unknown) {
    if (typeof n === 'string') {
      const matches = [...n.matchAll(PLACEHOLDER_REGEX)];
      for (const match of matches) {
        if (match[1]) placeholders.add(match[1]);
      }
      return;
    }
    if (Array.isArray(n)) {
      n.forEach(traverse);
      return;
    }
    if (isObject(n)) {
      Object.values(n).forEach(traverse);
      return;
    }
  }
  traverse(node);
  return Array.from(placeholders);
}

/**
 * Parses a template's full JSON structure to generate a list of form fields.
 */
export function parseTemplateForForm(templateData: unknown): FormField[] {
  if (!isObject(templateData)) {
    return [];
  }

  const fields = new Map<string, FormField>();
  const layers = Array.isArray(templateData.layers) ? templateData.layers : [];

  // --- Step 1: Handle special, known layer types with high priority ---
  for (const layer of layers) {
    if (isObject(layer)) {
      if (layer.type === 'text_carousel') {
        fields.set('texts', { name: 'texts', type: 'carousel', label: 'Carousel Text' });
      }
      if (layer.type === 'image' && layer.name === 'background') {
        fields.set('background', { name: 'background', type: 'background', label: 'Background Image' });
      }
      // The old `type: title` is now just a text field, so it will be caught in Step 2.
    }
  }

  // --- Step 2: Find all remaining placeholders in the entire template ---
  const allPlaceholders = findPlaceholdersInNode(templateData);

  for (const placeholder of allPlaceholders) {
    // If a field for this placeholder hasn't already been created, create a generic one.
    if (!fields.has(placeholder)) {
      
      // --- Improved Heuristic ---
      // Determine if this should be a textarea. We check if the placeholder name
      // exactly matches one of our keywords. This is more explicit than `includes`.
      const fieldType = TEXTAREA_KEYWORDS.has(placeholder.toLowerCase()) ? 'textarea' : 'text';

      fields.set(placeholder, {
        name: placeholder,
        type: fieldType,
        // Create a human-friendly label from the placeholder name
        label: placeholder.replace(/_/g, ' '),
      });
    }
  }

  return Array.from(fields.values());
}