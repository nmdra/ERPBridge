import { Button } from "./button";

export type PaginationProps = {
  label: string;
  firstItem: number;
  lastItem: number;
  totalItems: number;
  page: number;
  pageCount: number;
  onPrevious: () => void;
  onNext: () => void;
};

export function Pagination({
  label,
  firstItem,
  lastItem,
  totalItems,
  page,
  pageCount,
  onPrevious,
  onNext,
}: PaginationProps) {
  const canGoPrevious = page > 1;
  const canGoNext = page > 0 && page < pageCount;

  return (
    <nav
      aria-label={label}
      className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3 text-sm"
    >
      <p className="text-muted-foreground">
        Showing {firstItem}–{lastItem} of {totalItems}
      </p>
      <div className="flex items-center gap-2">
        <Button
          aria-label="Previous page"
          disabled={!canGoPrevious}
          onClick={onPrevious}
          type="button"
          variant="secondary"
        >
          Previous
        </Button>
        <p
          aria-atomic="true"
          aria-live="polite"
          className="text-muted-foreground"
        >
          Page {page} of {pageCount}
        </p>
        <Button
          aria-label="Next page"
          disabled={!canGoNext}
          onClick={onNext}
          type="button"
          variant="secondary"
        >
          Next
        </Button>
      </div>
    </nav>
  );
}
