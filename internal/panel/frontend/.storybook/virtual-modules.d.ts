declare module 'virtual:component-contracts' {
  /** Every component's `<!-- @component -->` contract, by component name. */
  const contracts: Record<string, string>;
  export default contracts;
}
