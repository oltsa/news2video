// Filename: components/layout/JobStatusDropdown.tsx
"use client";

import { useState, useEffect } from 'react';
import Link from 'next/link';
import * as api from '@/lib/api';
import { formatDistanceToNow } from 'date-fns';

import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Bell, CheckCircle2, XCircle, Loader, Download } from 'lucide-react';

const JobStatusIcon = ({ status }: { status: api.Job['status'] }) => {
  switch (status) {
    case 'complete': return <CheckCircle2 className="h-4 w-4 text-green-500" />;
    case 'failed': return <XCircle className="h-4 w-4 text-red-500" />;
    case 'rendering': return <Loader className="h-4 w-4 animate-spin text-blue-500" />;
    case 'queued':
    default:
      return <Loader className="h-4 w-4 animate-spin text-gray-400" />;
  }
};

export function JobStatusDropdown() {
  const [jobs, setJobs] = useState<api.Job[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchJobs = async () => {
      try {
        const response = await api.listMyJobs(10);
        setJobs(response.data || []);
      } catch (error) {
        console.error("Failed to fetch recent jobs", error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchJobs();
    const interval = setInterval(fetchJobs, 10000); // Poll every 10 seconds

    return () => clearInterval(interval);
  }, []);

  const inProgressJobs = jobs.filter(j => j.status === 'rendering' || j.status === 'queued').length;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="relative">
          <Bell className="h-5 w-5" />
          {inProgressJobs > 0 && (
            <span className="absolute top-1 right-1 flex h-3 w-3">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-sky-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-3 w-3 bg-sky-500"></span>
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <DropdownMenuLabel>Recent Render Jobs</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {isLoading ? (
          <DropdownMenuItem disabled>Loading...</DropdownMenuItem>
        ) : jobs.length === 0 ? (
          <DropdownMenuItem disabled>No recent jobs found.</DropdownMenuItem>
        ) : (
          jobs.map(job => (
            <DropdownMenuItem key={job.id} className="flex justify-between items-center gap-2">
              <div className="flex items-center gap-2 overflow-hidden">
                <JobStatusIcon status={job.status} />
                <div className="flex flex-col overflow-hidden">
                  <span className="font-medium truncate">{job.project_key}</span>
                  <span className="text-xs text-muted-foreground truncate">
                    {job.template_id} &middot; {formatDistanceToNow(new Date(job.created_at), { addSuffix: true })}
                  </span>
                </div>
              </div>
              {job.status === 'complete' && job.output_url && (
                <Button asChild variant="ghost" size="icon" className="h-7 w-7">
                  <a href={job.output_url} target="_blank" rel="noopener noreferrer"><Download className="h-4 w-4" /></a>
                </Button>
              )}
            </DropdownMenuItem>
          ))
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href="/jobs" className="w-full flex justify-center">View All Jobs</Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}