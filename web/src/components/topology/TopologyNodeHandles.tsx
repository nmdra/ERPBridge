import { Handle, Position } from "@xyflow/react";

import { handleIDs, type TopologyFlowNodeData } from "./topologyFlow";

const MULTI_PORT_THRESHOLD = 5;

function handleStyle(index: number, count: number) {
  if (count <= 1) return undefined;
  return { top: `${((index + 1) / (count + 1)) * 100}%` };
}

export function TopologyNodeHandles({ data }: { data: TopologyFlowNodeData }) {
  const handles = handleIDs(data.viewKind);
  const incoming = data.incomingPorts?.length
    ? data.incomingPorts
    : [handles.target];
  const outgoing = data.outgoingPorts?.length
    ? data.outgoingPorts
    : [handles.source];
  const manyPorts =
    incoming.length > MULTI_PORT_THRESHOLD ||
    outgoing.length > MULTI_PORT_THRESHOLD;

  return (
    <>
      {incoming.map((id, index) => (
        <Handle
          aria-label={`Incoming connection to ${data.label}`}
          className="!size-1.5 !border-background !bg-muted-foreground/50 !opacity-0"
          id={id}
          key={id}
          position={Position.Left}
          style={manyPorts ? handleStyle(index, incoming.length) : undefined}
          type="target"
        />
      ))}
      {outgoing.map((id, index) => (
        <Handle
          aria-label={`Outgoing connection from ${data.label}`}
          className="!size-1.5 !border-background !bg-muted-foreground/50 !opacity-0"
          id={id}
          key={id}
          position={Position.Right}
          style={manyPorts ? handleStyle(index, outgoing.length) : undefined}
          type="source"
        />
      ))}
    </>
  );
}
