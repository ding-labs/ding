import type { PlanDescription } from "../api/contracts";

// The preview's compiled rule is the authority for both local and hosted setup.
export function FirstWatchRule({ descriptions }: { descriptions: PlanDescription[] }) {
  return <div className="rule-summary">{descriptions.map(description => (
    <div key={description.id}>
      <p>{description.source}</p>
      <p>{description.condition}</p>
      <p>{description.policy}</p>
    </div>
  ))}</div>;
}
