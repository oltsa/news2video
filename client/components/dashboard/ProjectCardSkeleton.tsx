// Filename: components/dashboard/ProjectCardSkeleton.tsx
import { Card, CardHeader } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';

export function ProjectCardSkeleton() {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center gap-4">
        {/* Skeleton for the icon */}
        <Skeleton className="h-8 w-8 rounded-md" />
        <div className="grow space-y-2">
          {/* Skeleton for the title */}
          <Skeleton className="h-5 w-3/4" />
          {/* Skeleton for the description */}
          <Skeleton className="h-4 w-full" />
        </div>
      </CardHeader>
    </Card>
  );
}