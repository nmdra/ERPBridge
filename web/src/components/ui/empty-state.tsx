import { Inbox } from "lucide-react";

import { Card, CardContent } from "./card";

export function EmptyState({
  title,
  message,
}: {
  title: string;
  message: string;
}) {
  return (
    <Card>
      <CardContent className="flex flex-col items-center justify-center py-12 text-center">
        <Inbox aria-hidden="true" className="text-muted-foreground" size={28} />
        <h2 className="mt-3 font-medium">{title}</h2>
        <p className="mt-1 max-w-md text-sm text-muted-foreground">{message}</p>
      </CardContent>
    </Card>
  );
}
