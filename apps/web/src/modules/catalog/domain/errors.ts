/**
 * A name input that cannot be written as a library name. `field` names the input, with the
 * names of the golden vectors (contracts/naming-cases.json). Same rule as
 * internal/catalog/domain/errors.go.
 */
export class NameError extends Error {
  readonly field: string

  constructor(field: string) {
    super(`invalid name: ${field}`)
    this.name = 'NameError'
    this.field = field
  }
}
