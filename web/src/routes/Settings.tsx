import { Card, CardContent } from "../components/ui/card";

export function Settings() {
  return (
    <Card>
      <CardContent>
        <h2 className="font-medium">Console settings</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          Theme preferences are available in the top bar. The console keeps no
          persistent operational data.
        </p>
      </CardContent>
    </Card>
  );
}
