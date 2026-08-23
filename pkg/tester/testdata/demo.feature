# This file is part of REANA.
# Copyright (C) 2026 CERN.
#
# REANA is free software; you can redistribute it and/or modify it
# under the terms of the MIT License; see LICENSE file for more details.

Feature: ROOT6 workflow checks

    Scenario: The workflow start has produced the expected messages
        When the workflow is finished
        Then the engine logs should contain "Building DAG of jobs"

    Scenario: The generation step has produced the expected messages
        When the workflow is finished
        Then the job logs for the "gendata" step should contain
            """
            variables
            ---------
            (a0,a1,mean,nbkg,nsig,sig1frac,sigma1,x)
            """
        And the job logs for the "gendata" step should contain
            """
            datasets
            --------
            RooDataSet::modelData(x)
            """

    Scenario: The fitting step has produced the expected messages
        When the workflow is finished
        Then the job logs for the "fitdata" step should contain "MIGRAD MINIMIZATION HAS CONVERGED."

    Scenario: The workflow completion has produced the expected messages
        When the workflow is finished
        Then the logs should contain "3 of 3 steps (100%) done"

    Scenario: The workflow terminates in a reasonable amount of time
        When the workflow is finished
        Then the workflow run duration should be less than 10 minutes

    Scenario: The data generation step terminates in a reasonable amount of time
        When the workflow is finished
        Then the duration of the step "gendata" should be less than 8 minutes

    Scenario: The workspace contains all outputs
        When the workflow execution completes
        Then all the outputs should be included in the workspace

    Scenario: The workspace contains the expected files
        When the workflow status is "finished"
        Then the workspace should include "code/gendata.C"
        And "missing.txt" should not be in the workspace

    Scenario: The generated data has the expected size
        When the workflow is finished
        Then the size of the file "results/data.root" should be exactly 155KiB
        And the size of the file "results/data.root" should be between 150KiB and 160KiB

    Scenario: The workflow generates the correct final plot
        When the workflow is finished
        Then the file "results/message.txt" should contain "hello world"
        And the sha256 checksum of the file "results/message.txt" should be "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

    Scenario: The total workspace size remains within reasonable limits
        When the workflow is finished
        Then the workspace size should be more than 150KiB
        And the workspace size should be less than 20MiB
