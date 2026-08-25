import { EmptyState } from "../components/ui/empty-state";

export function Placeholder({
  title,
  message,
}: {
  title: string;
  message: string;
}) {
  return <EmptyState title={title} message={message} />;
}
