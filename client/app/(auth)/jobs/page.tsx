// Filename: app/(auth)/jobs/page.tsx
"use client";

import { useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import * as api from '@/lib/api';
import { format, formatDistanceToNow, isToday, isYesterday } from 'date-fns';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { AlertCircle, Download, ArrowLeft } from 'lucide-react';

const JobStatusBadge = ({ status }: { status: api.Job['status'] }) => {
  switch (status) {
    case 'complete': return <Badge variant="default" className="bg-green-600 hover:bg-green-600/80">Complete</Badge>;
    case 'failed': return <Badge variant="destructive">Failed</Badge>;
    case 'rendering': return <Badge variant="secondary" className="text-blue-500 border-blue-500">Rendering</Badge>;
    case 'queued':
    default:
      return <Badge variant="outline">Queued</Badge>;
  }
};

export default function JobsPage() {
  const [jobs, setJobs] = useState<api.Job[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const fetchJobs = useCallback(async () => {
    try {
      const response = await api.listMyJobs();
      setJobs(response.data || []);
    } catch (error) {
      console.error("Failed to fetch jobs", error);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchJobs();
    const interval = setInterval(fetchJobs, 15000);
    return () => clearInterval(interval);
  }, [fetchJobs]);

  const groupedJobs = jobs.reduce((acc, job) => {
    const date = new Date(job.created_at);
    let group: string;
    if (isToday(date)) group = 'Today';
    else if (isYesterday(date)) group = 'Yesterday';
    else group = format(date, 'MMMM d, yyyy');
    
    if (!acc[group]) acc[group] = [];
    acc[group].push(job);
    return acc;
  }, {} as Record<string, api.Job[]>);

  return (
    <div className="max-w-7xl mx-auto p-4 sm:p-6 lg:p-8 space-y-6">
      <div className="flex items-center gap-4">
        <Button asChild variant="outline" size="icon"><Link href="/dashboard"><ArrowLeft className="h-4 w-4" /></Link></Button>
        <h1 className="text-2xl font-bold">All Render Jobs</h1>
      </div>
      
      <TooltipProvider>
        <div className="space-y-8">
          {isLoading ? (
            <Skeleton className="h-64 w-full" />
          ) : Object.keys(groupedJobs).length === 0 ? (
            <p className="text-center text-muted-foreground py-10">You havent submitted any jobs yet.</p>
          ) : (
            Object.entries(groupedJobs).map(([group, jobsInGroup]) => (
              <div key={group}>
                <h2 className="text-lg font-semibold mb-2">{group}</h2>
                <div className="border rounded-lg bg-card">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-[120px]">Status</TableHead>
                        <TableHead>Project</TableHead>
                        <TableHead>Template</TableHead>
                        <TableHead>Submitted</TableHead>
                        <TableHead className="text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {jobsInGroup.map(job => (
                        <TableRow key={job.id}>
                          <TableCell><JobStatusBadge status={job.status} /></TableCell>
                          <TableCell className="font-medium">{job.project_key}</TableCell>
                          <TableCell>{job.template_id}</TableCell>
                          <TableCell>{formatDistanceToNow(new Date(job.created_at), { addSuffix: true })}</TableCell>
                          <TableCell className="text-right">
                            {job.status === 'complete' && job.output_url && (
                              <Button asChild variant="outline" size="sm">
                                <a href={job.output_url} target="_blank" rel="noopener noreferrer"><Download className="mr-2 h-4 w-4" />Download</a>
                              </Button>
                            )}
                            {/* +++ FIX: Check for .Valid and render .String +++ */}
                            {job.status === 'failed' && job.error_message?.Valid && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <AlertCircle className="h-5 w-5 text-destructive inline-block cursor-help" />
                                </TooltipTrigger>
                                <TooltipContent><p className="max-w-xs">{job.error_message.String}</p></TooltipContent>
                              </Tooltip>
                            )}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </div>
            ))
          )}
        </div>
      </TooltipProvider>
    </div>
  );
}